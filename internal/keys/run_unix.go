//go:build unix

package keys

import (
	"os/exec"
	"syscall"
)

// boundToProcessGroup puts cmd in a process group of its own and makes the
// deadline kill that whole group, not just cmd itself.
//
// Killing only the process it started is not enough to get control back. A tool
// that left a child running — an unlock agent, a helper it spawned — keeps the
// output pipe open, and whoever is reading that pipe waits for the child, not
// for the tool. That reader is not always SSHakku: when SSHakku is the
// SSH_ASKPASS helper, it is `ssh` itself, which would then wait out a child of
// a command it never ran. Killing the group ends the wait for everyone.
func boundToProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// Negative pid addresses the group. The process leads it (Setpgid
		// above), so its pid is the group id.
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
