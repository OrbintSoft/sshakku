//go:build unix

package agent

import "syscall"

// SysSignaler terminates a process with SIGTERM via the kernel.
type SysSignaler struct{}

// Terminate sends SIGTERM to pid.
func (SysSignaler) Terminate(pid int) error {
	// Signalling a live process is a kernel side effect with nothing to observe in
	// a unit test; the reaping logic that calls Signaler is tested with a fake.
	//coverage:ignore
	return syscall.Kill(pid, syscall.SIGTERM)
}

var _ Signaler = SysSignaler{}
