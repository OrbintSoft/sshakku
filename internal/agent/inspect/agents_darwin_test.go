//go:build darwin

package inspect

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// TestPlatformAgentsSysctlError covers platformAgents's enumeration-failure
// branch: when the kern.proc.all sysctl fails, the error is wrapped and
// returned rather than yielding a partial process list. The success path is
// exercised against the live sysctl by the real-agent integration tests.
func TestPlatformAgentsSysctlError(t *testing.T) {
	orig := sysctlProcList
	t.Cleanup(func() { sysctlProcList = orig })
	sysctlProcList = func(string, ...int) ([]unix.KinfoProc, error) {
		return nil, errors.New("boom")
	}

	_, err := (Inspector{}).Agents()
	assert.Error(t, err, "a failing sysctl must be reported, not yield a partial process list")
}

// TestAgentsReadsTheRealSysctl exercises the source an Inspector uses when it
// is given no tree of its own: the kern.proc.all sysctl. The fake-tree tests
// prove what is made of the process list; this one proves the list is found at
// all, against the live kernel rather than the stub above.
//
// It cannot require that any ssh-agent be running — a build machine may have
// none — so it asserts what holds either way: the scan succeeds, and nothing
// it reports is anything but an ssh-agent.
func TestAgentsReadsTheRealSysctl(t *testing.T) {
	procs, err := Inspector{}.Agents()
	require.NoError(t, err, "this machine's own process list must be readable")

	for _, p := range procs {
		require.NotEmptyf(t, p.Args, "pid %d was reported with no argv to have recognised it by", p.PID)
		assert.Equalf(t, "ssh-agent", filepath.Base(p.Args[0]),
			"pid %d is reported as an ssh-agent but was started as %q", p.PID, p.Args[0])
		assert.Positivef(t, p.PID, "the pid reported for %v", p.Args)
	}
}
