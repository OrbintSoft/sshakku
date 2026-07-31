//go:build !unix

package keys

import "os/exec"

// boundToProcessGroup is a no-op where process groups are not available; the
// deadline still kills the command itself, and Run's WaitDelay still bounds the
// wait for anything it left behind.
func boundToProcessGroup(*exec.Cmd) {}
