//go:build windows

package keys

import "os/exec"

// detachFromTerminal leaves cmd attached to whatever console it inherited:
// Windows has no session to leave, and detaching from a console is a different
// mechanism from Unix's setsid. What forces ssh-add to the SSH_ASKPASS helper
// here is the caller giving it no stdin, not the absence of a console.
func detachFromTerminal(*exec.Cmd) {}
