//go:build darwin

package agent

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
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
