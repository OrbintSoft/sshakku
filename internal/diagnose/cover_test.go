package diagnose

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/OrbintSoft/sshakku/internal/diagnose/hostcheck"
	"github.com/OrbintSoft/sshakku/internal/diagnose/launcher"
)

// fakeHostSource returns a fixed hostcheck.Checks, so Gather's host!=nil branch can be
// exercised without touching the real /proc or /sys.
type fakeHostSource struct{ hc hostcheck.Checks }

func (f fakeHostSource) Checks(context.Context) hostcheck.Checks { return f.hc }

// TestGatherRunsHostChecks covers Gather's branch that consults a non-nil
// hostcheck.Source and records its result in the report.
func TestGatherRunsHostChecks(t *testing.T) {
	enc := true
	r := Gather(t.Context(), Inputs{}, fakeSource{}, fakeProber{}, nil, nil, nil, fakeHostSource{hc: hostcheck.Checks{DiskEncrypted: &enc}})
	assert.Equal(t, &enc, r.Host.DiskEncrypted, "the report must carry what the host source answered")
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
			Ancestry: []launcher.ProcInfo{{PID: 9, Name: "ssh-agent"}, {PID: 8, Name: "bash"}},
		}},
		KeysErr: errors.New("permission denied"),
	})
	out := buf.String()
	// Three independent things the report has to say; assert so one run names
	// every one it left out.
	assert.Contains(t, out, "started by", "the report must say what started the agent")
	assert.Contains(t, out, "could not enumerate", "the report must say the keys could not be listed")
	assert.Contains(t, out, "(not answering)", "the report must say the socket in the environment is dead")
}

// TestHostChecksLineUndeterminedFields covers hostChecksLine's undetermined
// branches — a nil disk-encryption tri-state (triStateWord's nil case), a tmpfs
// /tmp of unknown size, a nil /tmp state, and nil secure hardware.
func TestHostChecksLineUndeterminedFields(t *testing.T) {
	tmpfs := true
	got := hostChecksLine(hostcheck.Checks{TmpTmpfs: &tmpfs})
	assert.Contains(t, got, "disk encryption: undetermined", "what was not established must be said to be undetermined")
	assert.Contains(t, got, "/tmp: tmpfs, size undetermined", "a tmpfs of unknown size is still reported as a tmpfs")
	assert.Contains(t, got, "secure hardware: undetermined", "what was not established must be said to be undetermined")

	enc := true
	assert.Contains(t, hostChecksLine(hostcheck.Checks{DiskEncrypted: &enc}), "/tmp: undetermined",
		"a /tmp nothing was learned about must be said to be undetermined")
}
