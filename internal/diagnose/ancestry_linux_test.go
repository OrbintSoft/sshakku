//go:build linux

package diagnose

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseStat(t *testing.T) {
	cases := []struct {
		name     string
		line     string
		wantComm string
		wantPPID int
		wantOK   bool
	}{
		{"simple", "3358 (ssh-agent) S 3300 3358 3300 0 -1", "ssh-agent", 3300, true},
		{"space in comm", "42 (sd pam) S 1 42", "sd pam", 1, true},
		{"parens in comm", "7 ((sd-pam)) S 1 7", "(sd-pam)", 1, true},
		{"init", "1 (systemd) S 0 1 1", "systemd", 0, true},
		{"no parens", "garbage", "", 0, false},
		// A stat file this malformed should never exist, but the guard that
		// rejects it is what keeps a doctor run from crashing on one: without
		// it the command name is cut from a negative range.
		{"parens the wrong way round", ") (", "", 0, false},
		{"truncated fields", "9 (x) S", "x", 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			comm, ppid, ok := parseStat([]byte(c.line))
			assert.Equal(t, c.wantOK, ok, "whether the line could be read at all")
			assert.Equal(t, c.wantComm, comm, "the command name between the parentheses")
			// A line that could not be read has no parent to have got wrong;
			// the cases above give it 0 to fill the field, not to be checked.
			if ok {
				assert.Equal(t, c.wantPPID, ppid, "the parent pid")
			}
		})
	}
}

func TestProcfsAncestryParent(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "77")
	require.NoError(t, os.MkdirAll(dir, 0o755), "lay out the fake /proc entry")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stat"), []byte("77 (ssh-agent) S 42 77 42"), 0o644), "write the fake stat file")

	ppid, name, ok := ProcfsAncestry{Root: root}.Parent(t.Context(), 77)
	assert.True(t, ok, "a process whose stat file is there must be answered for")
	assert.Equal(t, 42, ppid, "the parent read out of the stat file")
	assert.Equal(t, "ssh-agent", name, "the name read out of the stat file")

	_, _, ok = ProcfsAncestry{Root: root}.Parent(t.Context(), 999)
	assert.False(t, ok, "a process that is not there must not be answered for")
}

func TestLauncherLabel(t *testing.T) {
	_, ok := launcherLabel("nope")
	assert.False(t, ok, "a name the table does not carry must not be claimed as known")

	for _, comm := range []string{"systemd", "plasmashell", "sshd", "bash", "sddm-helper", "gnome-keyring-d"} {
		_, ok := launcherLabel(comm)
		assert.Truef(t, ok, "%s is a launcher the report must recognise", comm)
	}

	got, _ := launcherLabel("zsh")
	assert.Contains(t, got, "zsh", "the label for a login shell must name the shell it was")
}

func TestStartedBy(t *testing.T) {
	cases := []struct {
		name       string
		chain      []ProcInfo
		cgroupUnit string
		want       string
		ok         bool
	}{
		{"known launcher deeper", []ProcInfo{{9, "ssh-agent"}, {8, "dbus-daemon"}, {1, "systemd"}}, "", "systemd (user or system manager)", true},
		{"daemonized to init, no cgroup unit", []ProcInfo{{9, "ssh-agent"}, {1, "init"}}, "", "an unknown launcher (daemonized, reparented to init)", true},
		{"daemonized to init, cgroup unit found", []ProcInfo{{9, "ssh-agent"}, {1, "init"}}, "app-gpg-agent.service", "an unknown launcher (daemonized, reparented to init; systemd unit: app-gpg-agent.service)", true},
		{"immediate parent fallback", []ProcInfo{{9, "ssh-agent"}, {8, "weirdlauncher"}}, "", "weirdlauncher", true},
		{"too shallow", []ProcInfo{{9, "ssh-agent"}}, "", "", false},
		{"empty", nil, "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := startedBy(c.chain, c.cgroupUnit)
			assert.Equal(t, c.ok, ok, "whether anything could be said about who started it")
			assert.Equal(t, c.want, got, "what the report says started it")
		})
	}
}

func TestGatherReparentedToInitCgroupFallback(t *testing.T) {
	const foreign = "/tmp/foreign.sock"
	src := fakeSource{procs: []agent.AgentProc{
		{PID: 200, UID: 1000, Socket: foreign},
	}}
	prober := fakeProber{up: map[string]bool{foreign: true}}
	anc := fakeAncestry{
		200: {ppid: 1, name: "ssh-agent"},
		1:   {ppid: 0, name: "systemd"},
	}
	cg := fakeCgroup{200: "app-gpg-agent.service"}
	r := Gather(t.Context(), Inputs{FixedSock: fixed, LegacyDir: legacy, EnvSock: fixed, OurUID: 1000}, src, prober, anc, cg, nil, nil)

	assert.Truef(t, hasFinding(r, "systemd unit: app-gpg-agent.service"),
		"an agent whose parent is gone must still be named by its unit: %v", r.Findings)
}

// TestLauncherLabelDisplayManagers covers the display-manager and console-login
// cases of launcherLabel that the other tests don't reach.
func TestLauncherLabelDisplayManagers(t *testing.T) {
	for _, comm := range []string{"gdm", "lightdm", "login"} {
		_, ok := launcherLabel(comm)
		assert.Truef(t, ok, "%s is a launcher the report must recognise", comm)
	}
}

// TestParseCgroupUnitSingleColonLine covers parseCgroupUnit skipping a line that
// has no second colon, so there is no path field to scan for a unit.
func TestParseCgroupUnitSingleColonLine(t *testing.T) {
	_, ok := parseCgroupUnit([]byte("0:no-second-colon"))
	assert.False(t, ok, "a line with no path field names no unit")
}

// TestParseStatNonNumericPPID covers parseStat's branch where the ppid field is
// present but not a number.
func TestParseStatNonNumericPPID(t *testing.T) {
	comm, ppid, ok := parseStat([]byte("9 (x) S notanumber"))
	assert.False(t, ok, "a parent field that is not a number cannot be read")
	assert.Equal(t, "x", comm, "the command name is still what it was")
	assert.Zero(t, ppid, "no parent may be reported when none could be read")
}

// TestProcfsAncestryDefaultRoot covers Parent's empty-Root default to /proc.
func TestProcfsAncestryDefaultRoot(t *testing.T) {
	_, _, ok := ProcfsAncestry{}.Parent(t.Context(), 1<<30)
	assert.False(t, ok, "a pid that is not under /proc must not be answered for")
}

// TestProcfsAncestryDefaultRootIsProc pins which directory the empty Root falls
// back to, rather than only that a missing process is not answered for — which
// would hold for any wrong directory just as well. Every Linux process has a
// stat file of its own under /proc, so the test's own process is answered for
// exactly when the fallback is the real one.
func TestProcfsAncestryDefaultRootIsProc(t *testing.T) {
	ppid, name, ok := ProcfsAncestry{}.Parent(t.Context(), os.Getpid())
	require.True(t, ok, "the running process must be answered for")
	assert.Equal(t, os.Getppid(), ppid, "the parent must be this process's own")
	assert.NotEmpty(t, name, "the process must be named")
}

// TestProcfsAncestryParseFailure covers Parent's branch where the stat file is
// readable but unparseable.
func TestProcfsAncestryParseFailure(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "5")
	require.NoError(t, os.MkdirAll(dir, 0o755), "lay out the fake /proc entry")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stat"), []byte("no parentheses here"), 0o644), "write the malformed stat file")

	_, _, ok := ProcfsAncestry{Root: root}.Parent(t.Context(), 5)
	assert.False(t, ok, "a stat file that could not be read must not be answered from")
}

// TestProcfsAncestryReadError covers Parent's read-failure branch
// deterministically: a Root that does not exist makes the stat read fail
// regardless of whether the platform has a /proc at all.
func TestProcfsAncestryReadError(t *testing.T) {
	root := filepath.Join(t.TempDir(), "does-not-exist")
	_, _, ok := ProcfsAncestry{Root: root}.Parent(t.Context(), 1)
	assert.False(t, ok, "a stat file that could not be read must not be answered from")
}

// TestProcfsCgroupDefaultRoot covers Cgroup's empty-Root default to /proc.
func TestProcfsCgroupDefaultRoot(t *testing.T) {
	_, ok := ProcfsCgroup{}.Cgroup(1 << 30)
	assert.False(t, ok, "a pid that is not under /proc must not be answered for")
}
