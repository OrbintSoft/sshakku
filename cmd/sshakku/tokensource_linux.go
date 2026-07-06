//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// execTokenSource reads a target uid's socket token by re-executing this binary
// as a child running with that uid's credentials: a kernel-mediated
// fork+setuid+exec via SysProcAttr.Credential. It never changes this process's
// own credentials — no in-process setuid/seteuid, no thread-locking to reason
// about.
type execTokenSource struct{}

var _ TargetTokenSource = execTokenSource{}

// ReadToken requires the caller to already be root: only root can start a
// process under another uid's credentials.
func (execTokenSource) ReadToken(uid, gid int) (string, error) {
	if os.Geteuid() != 0 {
		return "", fmt.Errorf("reading uid %d's socket token requires root privileges (e.g. run via sudo)", uid)
	}
	self, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve own executable path: %w", err)
	}
	cmd := exec.Command(self, internalReadSocketTokenCmd)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)},
	}
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("read uid %d's socket token: %w", uid, err)
	}
	return strings.TrimSpace(string(out)), nil
}
