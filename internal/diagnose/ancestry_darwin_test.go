//go:build darwin

package diagnose

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// These verify F13 for the half of it macOS could not answer at all: until
// there was a PSAncestry, doctor had only a procfs walk, so on a Mac every
// agent was attributed to nobody and the report said so about all of them.

func TestParsePS(t *testing.T) {
	cases := []struct {
		name     string
		out      string
		wantPPID int
		wantName string
		wantOK   bool
	}{
		{"a plain command", " 501 zsh\n", 501, "zsh", true},
		{"a bundled app, whose comm is a full path", "  1 /Applications/Utilities/Terminal.app/Contents/MacOS/Terminal\n",
			1, "/Applications/Utilities/Terminal.app/Contents/MacOS/Terminal", true},
		// `ps -o comm=` reports the executable path, and an application bundle
		// may well sit in a directory with a space in it. Splitting on every
		// space would truncate the name to its first word.
		{"a path with a space in it", "42 /Applications/Visual Studio Code.app/Contents/MacOS/Electron\n",
			42, "/Applications/Visual Studio Code.app/Contents/MacOS/Electron", true},
		{"no such process", "", 0, "", false},
		{"a header but no row", "\n", 0, "", false},
		{"a ppid that is not a number", "notapid zsh\n", 0, "", false},
		{"a ppid with no command after it", "501\n", 0, "", false},
		// TrimSpace removes a trailing newline, so only a genuinely multi-line
		// answer exercises the cut — `ps` given several pids returns one row each,
		// and only the first is the parent asked about.
		{"more than one row", "  1 launchd\n 501 zsh\n", 1, "launchd", true},
		{"a ppid followed by only blanks", "501  \n", 0, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ppid, name, ok := parsePS([]byte(c.out))
			assert.Equal(t, c.wantOK, ok, "whether this output could be read at all")
			assert.Equal(t, c.wantPPID, ppid, "the parent pid read out of it")
			assert.Equal(t, c.wantName, name, "the command name read out of it")
		})
	}
}

// TestPSAncestryParent drives PSAncestry over the seam, so what is judged is
// how it reads an answer and what it does with a failure — not whether this
// machine happens to have the process it asked about.
func TestPSAncestryParent(t *testing.T) {
	t.Run("reads the answer", func(t *testing.T) {
		restore := psParent
		defer func() { psParent = restore }()
		psParent = func(pid int) ([]byte, error) {
			assert.Equal(t, 4242, pid, "the pid asked about must be the one the caller named")
			return []byte(" 1 /sbin/launchd\n"), nil
		}

		ppid, name, ok := PSAncestry{}.Parent(4242)
		assert.True(t, ok, "an answer that was read must be reported as one")
		assert.Equal(t, 1, ppid, "the parent pid in the answer")
		assert.Equal(t, "/sbin/launchd", name, "the command name in the answer")
	})

	t.Run("a failed read is an unknown parent, not a crash", func(t *testing.T) {
		restore := psParent
		defer func() { psParent = restore }()
		psParent = func(int) ([]byte, error) { return nil, errors.New("ps: no such process") }

		_, _, ok := PSAncestry{}.Parent(4242)
		assert.False(t, ok, "a ps that failed must leave the parent unknown, not invent one")
	})
}

func TestLauncherLabel(t *testing.T) {
	_, ok := launcherLabel("nope")
	assert.False(t, ok, "a name the table does not carry must not be claimed as known")

	for _, comm := range []string{"/sbin/launchd", "launchd", "/usr/libexec/loginwindow", "sshd", "login", "zsh"} {
		_, ok := launcherLabel(comm)
		assert.Truef(t, ok, "%s is a launcher the report must recognise", comm)
	}

	shell, _ := launcherLabel("zsh")
	assert.Contains(t, shell, "zsh", "the label for a login shell must name the shell it was")

	// Matched by suffix, because what `ps` reports is wherever the bundle is
	// installed, which is not always /Applications.
	got, ok := launcherLabel("/Users/someone/Applications/iTerm.app/Contents/MacOS/iTerm2")
	assert.True(t, ok, "a bundle installed outside /Applications is still that application")
	assert.Equal(t, "iTerm2", got, "the application the bundle path names")
}

// TestReparentedLabel pins what can be said when the trail ends: on macOS
// nothing survives a double-fork that would still name the launcher, so the
// answer must not pretend otherwise — and must not name systemd, which is what
// the shared wording used to do on every platform.
func TestReparentedLabel(t *testing.T) {
	got := reparentedLabel("")
	assert.Contains(t, got, "launchd", "on macOS the trail ends at launchd, and the report says so")
	assert.NotContains(t, got, "systemd", "macOS runs no systemd, so the report must not name one")
	assert.Equal(t, got, reparentedLabel("app-something.service"),
		"there are no control groups here, so a unit name is nothing to go on")
}

// TestStartedByOnDarwin checks the shared attribution walk against this
// platform's own labels: the same rule, a different table.
func TestStartedByOnDarwin(t *testing.T) {
	cases := []struct {
		name  string
		chain []ProcInfo
		want  string
		ok    bool
	}{
		{"a known launcher deeper up", []ProcInfo{{9, "ssh-agent"}, {8, "somehelper"}, {1, "/sbin/launchd"}},
			"launchd (the system or per-user manager)", true},
		{"daemonized, reparented", []ProcInfo{{9, "ssh-agent"}, {1, "launchd"}},
			"an unknown launcher (daemonized, reparented to launchd)", true},
		{"nothing recognised falls back to the parent", []ProcInfo{{9, "ssh-agent"}, {8, "weirdlauncher"}},
			"weirdlauncher", true},
		{"too shallow to attribute", []ProcInfo{{9, "ssh-agent"}}, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := startedBy(c.chain, "")
			assert.Equal(t, c.ok, ok, "whether anything could be said about who started it")
			assert.Equal(t, c.want, got, "what the report says started it")
		})
	}
}

func TestNoCgroupsReportsNothing(t *testing.T) {
	unit, ok := NoCgroups{}.Cgroup(1)
	assert.False(t, ok, "a system with no control groups can answer for no process")
	assert.Empty(t, unit, "there is no unit to name")
}
