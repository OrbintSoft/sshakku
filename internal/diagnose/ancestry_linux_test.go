//go:build linux

package diagnose

import (
	"github.com/OrbintSoft/sshakku/internal/agent"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
		{"truncated fields", "9 (x) S", "x", 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			comm, ppid, ok := parseStat([]byte(c.line))
			if ok != c.wantOK || comm != c.wantComm || (ok && ppid != c.wantPPID) {
				t.Errorf("parseStat(%q) = (%q,%d,%v), want (%q,%d,%v)",
					c.line, comm, ppid, ok, c.wantComm, c.wantPPID, c.wantOK)
			}
		})
	}
}

func TestProcfsAncestryParent(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "77")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stat"), []byte("77 (ssh-agent) S 42 77 42"), 0o644); err != nil {
		t.Fatal(err)
	}
	ppid, name, ok := ProcfsAncestry{Root: root}.Parent(77)
	if !ok || ppid != 42 || name != "ssh-agent" {
		t.Errorf("Parent(77) = (%d,%q,%v), want (42,ssh-agent,true)", ppid, name, ok)
	}
	if _, _, ok := (ProcfsAncestry{Root: root}).Parent(999); ok {
		t.Error("Parent(999) reported ok for a missing process")
	}
}

func TestLauncherLabel(t *testing.T) {
	if _, ok := launcherLabel("nope"); ok {
		t.Error("launcherLabel(nope) reported known")
	}
	for _, comm := range []string{"systemd", "plasmashell", "sshd", "bash", "sddm-helper", "gnome-keyring-d"} {
		if _, ok := launcherLabel(comm); !ok {
			t.Errorf("launcherLabel(%q) not recognised", comm)
		}
	}
	if got, _ := launcherLabel("zsh"); !strings.Contains(got, "zsh") {
		t.Errorf("login-shell label = %q, want it to name zsh", got)
	}
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
			if ok != c.ok || got != c.want {
				t.Errorf("startedBy(%v, %q) = (%q,%v), want (%q,%v)", c.chain, c.cgroupUnit, got, ok, c.want, c.ok)
			}
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
	r := Gather(Inputs{FixedSock: fixed, LegacyDir: legacy, EnvSock: fixed, OurUID: 1000}, src, prober, anc, cg, nil, nil)

	if !hasFinding(r, "systemd unit: app-gpg-agent.service") {
		t.Errorf("findings = %v, want the cgroup-fallback unit named", r.Findings)
	}
}

// TestLauncherLabelDisplayManagers covers the display-manager and console-login
// cases of launcherLabel that the other tests don't reach.
func TestLauncherLabelDisplayManagers(t *testing.T) {
	for _, comm := range []string{"gdm", "lightdm", "login"} {
		if _, ok := launcherLabel(comm); !ok {
			t.Errorf("launcherLabel(%q) not recognised", comm)
		}
	}
}

// TestParseCgroupUnitSingleColonLine covers parseCgroupUnit skipping a line that
// has no second colon, so there is no path field to scan for a unit.
func TestParseCgroupUnitSingleColonLine(t *testing.T) {
	if unit, ok := parseCgroupUnit([]byte("0:no-second-colon")); ok {
		t.Errorf("parseCgroupUnit = (%q,%v), want no unit", unit, ok)
	}
}

// TestParseStatNonNumericPPID covers parseStat's branch where the ppid field is
// present but not a number.
func TestParseStatNonNumericPPID(t *testing.T) {
	comm, ppid, ok := parseStat([]byte("9 (x) S notanumber"))
	if ok || comm != "x" || ppid != 0 {
		t.Errorf("parseStat = (%q,%d,%v), want (\"x\",0,false)", comm, ppid, ok)
	}
}

// TestProcfsAncestryDefaultRoot covers Parent's empty-Root default to /proc.
func TestProcfsAncestryDefaultRoot(t *testing.T) {
	if _, _, ok := (ProcfsAncestry{}).Parent(1 << 30); ok {
		t.Error("Parent of a nonexistent pid under /proc must report not-ok")
	}
}

// TestProcfsAncestryParseFailure covers Parent's branch where the stat file is
// readable but unparseable.
func TestProcfsAncestryParseFailure(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "5")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stat"), []byte("no parentheses here"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := (ProcfsAncestry{Root: root}).Parent(5); ok {
		t.Error("Parent of a malformed stat must report not-ok")
	}
}

// TestProcfsAncestryReadError covers Parent's read-failure branch
// deterministically: a Root that does not exist makes the stat read fail
// regardless of whether the platform has a /proc at all.
func TestProcfsAncestryReadError(t *testing.T) {
	root := filepath.Join(t.TempDir(), "does-not-exist")
	if _, _, ok := (ProcfsAncestry{Root: root}).Parent(1); ok {
		t.Error("Parent with an unreadable stat must report not-ok")
	}
}

// TestProcfsCgroupDefaultRoot covers Cgroup's empty-Root default to /proc.
func TestProcfsCgroupDefaultRoot(t *testing.T) {
	if _, ok := (ProcfsCgroup{}).Cgroup(1 << 30); ok {
		t.Error("Cgroup of a nonexistent pid under /proc must report not-ok")
	}
}
