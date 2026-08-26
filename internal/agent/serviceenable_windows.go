//go:build windows

package agent

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/sys/windows"
)

// EnableAgentService lets the agent's service be started again, on a system
// where somebody has disabled it.
//
// This writes, and what it writes belongs to the machine rather than to one
// account: a service disabled for everybody is enabled for everybody. That is
// why nothing on the login path calls it — a shell opening is not somebody
// asking for the machine to be changed — and why the diagnostic tool, which is
// the one part of this program meant to be run with an administrator's
// privileges, is where it lives.
func EnableAgentService(ctx context.Context) error { return systemService{}.enable(ctx) }

// enable sets the service back to starting by itself, which is what it is set
// to on a system where nobody has changed it.
//
// Every other field is left as it stands: this asks for one thing, and a call
// that rewrote the binary path or the account a service runs as, on the way to
// enabling it, would be a far larger thing than the report offered to do.
func (s systemService) enable(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.withServiceHandle(windows.SERVICE_CHANGE_CONFIG, func(handle windows.Handle) error {
		err := windows.ChangeServiceConfig(handle,
			windows.SERVICE_NO_CHANGE, windows.SERVICE_AUTO_START, windows.SERVICE_NO_CHANGE,
			nil, nil, nil, nil, nil, nil, nil)
		if err != nil {
			return s.explainEnabling(err)
		}
		return nil
	}, s.explainEnabling)
}

// explainEnabling turns what the service manager said about an attempt to
// enable the service into a sentence somebody can act on.
//
// It is not the same sentence as a refusal to start one, and the difference is
// the point. What puts a refused start right is a command an administrator runs
// against the service; what puts a refused enable right is this same command,
// run again from a session that has the privileges it is meant to be given —
// so that is what it says, rather than sending somebody to a second tool to do
// what the one in front of them does.
func (s systemService) explainEnabling(err error) error {
	name := s.name()
	switch {
	case errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST):
		return fmt.Errorf("this system has no %s service to enable; "+
			"an administrator can add it with: Add-WindowsCapability -Online -Name OpenSSH.Client~~~~0.0.1.0", name)
	case errors.Is(err, windows.ERROR_ACCESS_DENIED):
		return fmt.Errorf("this session may not enable the %s service; "+
			"run sshakku doctor --fix again from an administrator session, which is what it is for", name)
	default:
		return fmt.Errorf("enabling the %s service: %w", name, err)
	}
}
