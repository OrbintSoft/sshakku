//go:build windows

package agent

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/sys/windows"
)

// agentServiceName is what this system's service manager knows the agent by.
// The name belongs to the program that installs the service, not to us.
const agentServiceName = "ssh-agent"

// systemService drives the agent's service through this system's own service
// manager.
type systemService struct {
	// Name overrides the service to drive; empty means the one above. It is
	// what lets the refusals below be exercised against a service that cannot
	// exist, rather than only against whatever this machine happens to have.
	Name string
}

// name is the service this instance drives.
func (s systemService) name() string {
	if s.Name != "" {
		return s.Name
	}
	return agentServiceName
}

// State reports what the agent's service is doing.
func (s systemService) State(ctx context.Context) (serviceState, error) {
	if err := ctx.Err(); err != nil {
		return serviceSomethingElse, err
	}
	state := serviceSomethingElse
	err := s.withService(func(handle windows.Handle) error {
		var status windows.SERVICE_STATUS
		if err := windows.QueryServiceStatus(handle, &status); err != nil {
			return fmt.Errorf("asking what the %s service is doing: %w", s.name(), err)
		}
		state = stateOf(status.CurrentState)
		return nil
	})
	return state, err
}

// Start asks the service manager to start the agent's service. A service
// somebody else started in the meantime is what was wanted, so that is not
// reported as a failure.
func (s systemService) Start(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.withService(func(handle windows.Handle) error {
		if err := windows.StartService(handle, 0, nil); err != nil {
			return s.explain(err)
		}
		return nil
	})
}

// withService opens the agent's service and hands it to do.
//
// It asks for exactly the two rights used — reading the state, and starting —
// because that is what an ordinary account is granted, and an ordinary account
// is who this is for. A handle opened for everything, which is what a
// general-purpose service library asks for, is refused for anyone who is not
// an administrator, and the refusal would look like the service being out of
// reach rather than like too much having been asked for.
func (s systemService) withService(do func(windows.Handle) error) error {
	manager, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return fmt.Errorf("reaching this system's service manager: %w", err)
	}
	defer func() { _ = windows.CloseServiceHandle(manager) }()

	wide, err := windows.UTF16PtrFromString(s.name())
	if err != nil {
		return fmt.Errorf("naming the %s service: %w", s.name(), err)
	}
	service, err := windows.OpenService(manager, wide,
		windows.SERVICE_QUERY_STATUS|windows.SERVICE_START)
	if err != nil {
		return s.explain(err)
	}
	defer func() { _ = windows.CloseServiceHandle(service) }()

	return do(service)
}

// explain turns what the service manager said into a sentence the person in
// front of the shell can act on, naming the command that puts it right. An
// exit status on its own names nothing anybody can do anything about, and
// these three are the whole of what stands between an account and its agent.
func (s systemService) explain(err error) error {
	name := s.name()
	switch {
	case errors.Is(err, windows.ERROR_SERVICE_ALREADY_RUNNING):
		return nil
	case errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST):
		return fmt.Errorf("this system has no %s service, so there is no agent to reach; "+
			"an administrator can add it with: Add-WindowsCapability -Online -Name OpenSSH.Client~~~~0.0.1.0", name)
	case errors.Is(err, windows.ERROR_SERVICE_DISABLED):
		return fmt.Errorf("the %s service is disabled, so nothing can start it from here; "+
			"an administrator can enable it with: Set-Service %s -StartupType Automatic", name, name)
	case errors.Is(err, windows.ERROR_ACCESS_DENIED):
		return fmt.Errorf("this account may not start the %s service; "+
			"an administrator can start it with: Start-Service %s", name, name)
	default:
		return fmt.Errorf("starting the %s service: %w", name, err)
	}
}

// stateOf reads the service manager's answer as one of the states the
// lifecycle acts on. Anything else — pausing, stopping, a state this system
// adds later — is somethingElse, which is waited on rather than acted on.
func stateOf(current uint32) serviceState {
	switch current {
	case windows.SERVICE_RUNNING:
		return serviceRunning
	case windows.SERVICE_STOPPED:
		return serviceStopped
	case windows.SERVICE_START_PENDING:
		return serviceStarting
	default:
		return serviceSomethingElse
	}
}

var _ serviceControl = systemService{}
