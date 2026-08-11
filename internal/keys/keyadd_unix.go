//go:build unix

package keys

import (
	"os/exec"
	"syscall"
)

// detachFromTerminal gives cmd a session of its own, so ssh-add has no
// controlling terminal to ask on and must go to the SSH_ASKPASS helper for the
// passphrase instead.
func detachFromTerminal(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
