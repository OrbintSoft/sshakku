//go:build windows

package agent

import "errors"

// errNoTerminate is what SysSignaler reports here. Terminate is asked for a
// graceful stop — SIGTERM, which a process can act on — and Windows has no
// equivalent to send an arbitrary process: what it offers is TerminateProcess,
// which is not the same request and is not one to make of an agent holding
// keys without having decided to.
var errNoTerminate = errors.New("terminating an agent is not implemented on windows")

// SysSignaler terminates a process.
type SysSignaler struct{}

// Terminate reports errNoTerminate.
func (SysSignaler) Terminate(int) error { return errNoTerminate }

var _ Signaler = SysSignaler{}
