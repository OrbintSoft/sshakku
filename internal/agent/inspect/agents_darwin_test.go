//go:build darwin

package inspect

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// kinfo builds the one thing platformAgents reads out of a kern.proc.all
// entry: which process it is and who owns it. Everything else the kernel
// returns is ignored, and a literal here says so.
func kinfo(pid int32, uid uint32) unix.KinfoProc {
	var kp unix.KinfoProc
	kp.Proc.P_pid = pid
	kp.Eproc.Pcred.P_ruid = uid
	return kp
}

// procArgsBuffer builds a buffer shaped like kern.procargs2's answer for argv.
func procArgsBuffer(t *testing.T, argv []string) []byte {
	t.Helper()
	return buildKernProcArgs2("/usr/bin/"+argv[0], 0, argv)
}

// stubSysctls points both sysctls at the answers this test chooses: procs is
// the process list, argv is what each pid's arguments read back as.
func stubSysctls(t *testing.T, procs []unix.KinfoProc, argv map[int][]string) {
	t.Helper()
	origList, origArgs := sysctlProcList, sysctlProcArgs
	t.Cleanup(func() { sysctlProcList, sysctlProcArgs = origList, origArgs })

	sysctlProcList = func(string, ...int) ([]unix.KinfoProc, error) { return procs, nil }
	sysctlProcArgs = func(_ string, args ...int) ([]byte, error) {
		a, ok := argv[args[0]]
		if !ok {
			return nil, errors.New("no such process")
		}
		return procArgsBuffer(t, a), nil
	}
}

// TestPlatformAgentsReportsTheAgentsTheKernelNamed checks what this platform
// makes of the kernel's answers: which processes it reports and what it says
// about each. It is the mapping, and nothing else here decides it.
func TestPlatformAgentsReportsTheAgentsTheKernelNamed(t *testing.T) {
	stubSysctls(t,
		[]unix.KinfoProc{
			kinfo(100, 501),
			kinfo(200, 502),
			kinfo(300, 501),
			kinfo(400, 501),
		},
		map[int][]string{
			100: {"ssh-agent", "-a", "/tmp/ours.sock"},
			200: {"ssh-agent", "-a/tmp/joined.sock"},
			300: {"bash", "-l"},
			// 400 answers no argv at all: gone, or another user's.
		})

	procs, err := (Inspector{}).Agents()
	require.NoError(t, err, "Agents")

	require.Len(t, procs, 2, "only the two ssh-agents may be reported, of the four the kernel named")
	assert.Equal(t, 100, procs[0].PID, "the pid must come from the process list entry")
	assert.Equal(t, 501, procs[0].UID, "and the owner from that entry's real uid, which is what gates reaping")
	assert.Equal(t, "/tmp/ours.sock", procs[0].Socket, "the bind address must be read out of the argv")
	assert.Equal(t, []string{"ssh-agent", "-a", "/tmp/ours.sock"}, procs[0].Args, "and the argv kept whole")

	assert.Equal(t, 200, procs[1].PID, "pid")
	assert.Equal(t, 502, procs[1].UID, "each agent's own owner, not the first one's")
	assert.Equal(t, "/tmp/joined.sock", procs[1].Socket, "the joined -a form is the same address")
}

// TestPlatformAgentsSkipsPidZero covers the guard on the process list itself:
// the kernel names entries no pid can be read from, and asking the argv sysctl
// about one is asking about nothing.
func TestPlatformAgentsSkipsPidZero(t *testing.T) {
	stubSysctls(t, []unix.KinfoProc{kinfo(0, 0)}, map[int][]string{
		0: {"ssh-agent", "-a", "/tmp/zero.sock"},
	})

	procs, err := (Inspector{}).Agents()
	require.NoError(t, err, "Agents")
	assert.Empty(t, procs, "a process with no pid cannot be signalled or probed, so it is not one to report")
}

// TestPlatformAgentsSysctlError covers platformAgents's enumeration-failure
// branch: when the kern.proc.all sysctl fails, the error is wrapped and
// returned rather than yielding a partial process list.
func TestPlatformAgentsSysctlError(t *testing.T) {
	orig := sysctlProcList
	t.Cleanup(func() { sysctlProcList = orig })
	sysctlProcList = func(string, ...int) ([]unix.KinfoProc, error) {
		return nil, errors.New("boom")
	}

	_, err := (Inspector{}).Agents()
	assert.Error(t, err, "a failing sysctl must be reported, not yield a partial process list")
}
