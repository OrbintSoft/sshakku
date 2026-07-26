package diagnose

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// fakeHostSource returns a fixed HostChecks, so Gather's host!=nil branch can be
// exercised without touching the real /proc or /sys.
type fakeHostSource struct{ hc HostChecks }

func (f fakeHostSource) Checks() HostChecks { return f.hc }

// TestGatherRunsHostChecks covers Gather's branch that consults a non-nil
// HostSource and records its result in the report.
func TestGatherRunsHostChecks(t *testing.T) {
	enc := true
	r := Gather(Inputs{}, fakeSource{}, fakeProber{}, nil, nil, nil, fakeHostSource{hc: HostChecks{DiskEncrypted: &enc}})
	if r.Host.DiskEncrypted == nil || !*r.Host.DiskEncrypted {
		t.Errorf("Report.Host.DiskEncrypted = %v, want the host source's true", r.Host.DiskEncrypted)
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

// TestParseStatNonNumericPPID covers parseStat's branch where the ppid field is
// present but not a number.
func TestParseStatNonNumericPPID(t *testing.T) {
	comm, ppid, ok := parseStat([]byte("9 (x) S notanumber"))
	if ok || comm != "x" || ppid != 0 {
		t.Errorf("parseStat = (%q,%d,%v), want (\"x\",0,false)", comm, ppid, ok)
	}
}

// TestParseCgroupUnitSingleColonLine covers parseCgroupUnit skipping a line that
// has no second colon, so there is no path field to scan for a unit.
func TestParseCgroupUnitSingleColonLine(t *testing.T) {
	if unit, ok := parseCgroupUnit([]byte("0:no-second-colon")); ok {
		t.Errorf("parseCgroupUnit = (%q,%v), want no unit", unit, ok)
	}
}

// TestFormatRemainingBranches covers Format's attribution line ("started by …"),
// its key-enumeration-error line, and the "not answering" SSH_AUTH_SOCK suffix,
// which the other Format tests don't exercise.
func TestFormatRemainingBranches(t *testing.T) {
	var buf bytes.Buffer
	Format(&buf, Report{
		EnvSock:      "/run/user/1000/ssh",
		EnvReachable: false,
		Agents: []AgentView{{
			PID:      9,
			Ancestry: []ProcInfo{{PID: 9, Name: "ssh-agent"}, {PID: 8, Name: "bash"}},
		}},
		KeysErr: errors.New("permission denied"),
	})
	out := buf.String()
	for _, want := range []string{"started by", "could not enumerate", "(not answering)"} {
		if !strings.Contains(out, want) {
			t.Errorf("Format output missing %q\n%s", want, out)
		}
	}
}

// TestHostChecksLineUndeterminedFields covers hostChecksLine's undetermined
// branches — a nil disk-encryption tri-state (triStateWord's nil case), a tmpfs
// /tmp of unknown size, a nil /tmp state, and nil secure hardware.
func TestHostChecksLineUndeterminedFields(t *testing.T) {
	tmpfs := true
	got := hostChecksLine(HostChecks{TmpTmpfs: &tmpfs})
	for _, want := range []string{"disk encryption: undetermined", "/tmp: tmpfs, size undetermined", "secure hardware: undetermined"} {
		if !strings.Contains(got, want) {
			t.Errorf("hostChecksLine = %q, want it to contain %q", got, want)
		}
	}
	enc := true
	if got := hostChecksLine(HostChecks{DiskEncrypted: &enc}); !strings.Contains(got, "/tmp: undetermined") {
		t.Errorf("hostChecksLine = %q, want it to contain %q", got, "/tmp: undetermined")
	}
}
