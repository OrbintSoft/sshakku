//go:build windows

package keys

import "os/exec"

// boundToProcessGroup leaves cmd as it is: Windows has no process group a
// single kill can address, and terminating a tree here means a job object,
// which this package does not yet create. The consequence is that a tool which
// left a child behind is not itself ended by the deadline — the child keeps the
// output pipe open and only the command is killed. What still bounds the wait
// the caller experiences is Run's WaitDelay, which gives up on the pipe rather
// than on the process.
func boundToProcessGroup(*exec.Cmd) {}
