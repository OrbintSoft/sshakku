//go:build darwin

package diagnose

import (
	"errors"
	"strings"
	"testing"
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
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ppid, name, ok := parsePS([]byte(c.out))
			if ok != c.wantOK || ppid != c.wantPPID || name != c.wantName {
				t.Errorf("parsePS(%q) = (%d,%q,%v), want (%d,%q,%v)",
					c.out, ppid, name, ok, c.wantPPID, c.wantName, c.wantOK)
			}
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
			if pid != 4242 {
				t.Errorf("asked about pid %d, want 4242", pid)
			}
			return []byte(" 1 /sbin/launchd\n"), nil
		}

		ppid, name, ok := PSAncestry{}.Parent(4242)
		if !ok || ppid != 1 || name != "/sbin/launchd" {
			t.Errorf("Parent = (%d,%q,%v), want (1,\"/sbin/launchd\",true)", ppid, name, ok)
		}
	})

	t.Run("a failed read is an unknown parent, not a crash", func(t *testing.T) {
		restore := psParent
		defer func() { psParent = restore }()
		psParent = func(int) ([]byte, error) { return nil, errors.New("ps: no such process") }

		if _, _, ok := (PSAncestry{}).Parent(4242); ok {
			t.Error("a ps that failed must report the parent as unknown")
		}
	})
}

func TestLauncherLabel(t *testing.T) {
	if _, ok := launcherLabel("nope"); ok {
		t.Error("launcherLabel(nope) reported known")
	}
	for _, comm := range []string{"/sbin/launchd", "launchd", "/usr/libexec/loginwindow", "sshd", "login", "zsh"} {
		if _, ok := launcherLabel(comm); !ok {
			t.Errorf("launcherLabel(%q) not recognised", comm)
		}
	}
	if got, _ := launcherLabel("zsh"); !strings.Contains(got, "zsh") {
		t.Errorf("login-shell label = %q, want it to name zsh", got)
	}
	// Matched by suffix, because what `ps` reports is wherever the bundle is
	// installed, which is not always /Applications.
	got, ok := launcherLabel("/Users/someone/Applications/iTerm.app/Contents/MacOS/iTerm2")
	if !ok || got != "iTerm2" {
		t.Errorf("a bundle outside /Applications = (%q,%v), want iTerm2", got, ok)
	}
}

// TestReparentedLabel pins what can be said when the trail ends: on macOS
// nothing survives a double-fork that would still name the launcher, so the
// answer must not pretend otherwise — and must not name systemd, which is what
// the shared wording used to do on every platform.
func TestReparentedLabel(t *testing.T) {
	got := reparentedLabel("")
	if !strings.Contains(got, "launchd") {
		t.Errorf("reparented label = %q, want it to name launchd", got)
	}
	if strings.Contains(got, "systemd") {
		t.Errorf("reparented label = %q, want no mention of systemd on macOS", got)
	}
	if reparentedLabel("app-something.service") != got {
		t.Error("there is no cgroup on macOS, so a unit name must change nothing")
	}
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
			if ok != c.ok || got != c.want {
				t.Errorf("startedBy(%v) = (%q,%v), want (%q,%v)", c.chain, got, ok, c.want, c.ok)
			}
		})
	}
}

func TestNoCgroupsReportsNothing(t *testing.T) {
	if unit, ok := (NoCgroups{}).Cgroup(1); ok || unit != "" {
		t.Errorf("Cgroup = (%q,%v), want nothing on a system with no control groups", unit, ok)
	}
}
