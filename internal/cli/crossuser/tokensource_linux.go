//go:build linux

package crossuser

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// needsRootError is a cross-user token read attempted without the privilege it
// requires. It carries the uid asked about, because that is what makes the
// message tell one refusal from another in a log.
type needsRootError struct{ uid int }

func (e needsRootError) Error() string {
	return fmt.Sprintf("reading uid %d's socket token requires root privileges (e.g. run via sudo)", e.uid)
}

// Exec reads a target uid's socket token by re-executing this binary
// as a child running with that uid's credentials: a kernel-mediated
// fork+setuid+exec via SysProcAttr.Credential. It never changes this process's
// own credentials — no in-process setuid/seteuid, no thread-locking to reason
// about.
type Exec struct{}

var _ Source = Exec{}

// ReadToken requires the caller to already be root: only root can start a
// process under another uid's credentials.
func (Exec) ReadToken(ctx context.Context, uid, gid int) (string, error) {
	if os.Geteuid() != 0 {
		return "", needsRootError{uid: uid}
	}
	// Everything below performs the privileged fork+setuid+exec, runnable only
	// as real root with a second uid to assume, so it cannot run in a unit test;
	// the euid guard above stays unit-tested. Each straight-line block carries
	// its own ignore marker because go-ignore-cov scopes per block.
	//coverage:ignore
	self, err := os.Executable()
	if err != nil {
		//coverage:ignore
		return "", fmt.Errorf("resolve own executable path: %w", err)
	}
	//coverage:ignore
	cmd := exec.CommandContext(ctx, self, ReadSocketTokenCmd)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)},
	}
	out, err := cmd.Output()
	if err != nil {
		//coverage:ignore
		return "", fmt.Errorf("read uid %d's socket token: %w", uid, err)
	}
	//coverage:ignore
	return strings.TrimSpace(string(out)), nil
}
