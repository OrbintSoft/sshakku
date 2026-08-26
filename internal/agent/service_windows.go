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
	err := s.withService(windows.SERVICE_QUERY_STATUS, func(handle windows.Handle) error {
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
	return s.withService(windows.SERVICE_START, func(handle windows.Handle) error {
		if err := windows.StartService(handle, 0, nil); err != nil {
			return s.explain(err)
		}
		return nil
	})
}

// withService opens the agent's service for the given access and hands it to
// do.
//
// The access is the caller's to name, and each names exactly the one right it
// uses, because that is how an ordinary account is granted them: the service's
// own security descriptor hands out reading its state, reading its
// configuration and starting it separately, and reserves changing it for
// administrators. A handle opened for everything, which is what a
// general-purpose service library asks for, is refused for anyone who is not
// an administrator, and the refusal would look like the service being out of
// reach rather than like too much having been asked for.
func (s systemService) withService(access uint32, do func(windows.Handle) error) error {
	return s.withServiceHandle(access, do, s.explain)
}

// withServiceHandle opens the agent's service for the given access and hands it
// to do, turning a refusal into a sentence through explaining.
//
// Which sentence that is belongs to the caller, because a refusal only means
// something together with what was being attempted: an account that may not
// change a service's configuration may perfectly well start it, and the two are
// put right by different things.
func (s systemService) withServiceHandle(
	access uint32,
	do func(windows.Handle) error,
	explaining func(error) error,
) error {
	manager, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return fmt.Errorf("reaching this system's service manager: %w", err)
	}
	defer func() { _ = windows.CloseServiceHandle(manager) }()

	wide, err := windows.UTF16PtrFromString(s.name())
	if err != nil {
		return fmt.Errorf("naming the %s service: %w", s.name(), err)
	}
	service, err := windows.OpenService(manager, wide, access)
	if err != nil {
		return explaining(err)
	}
	defer func() { _ = windows.CloseServiceHandle(service) }()

	return do(service)
}

// The three states that stand between an account and its agent, each carrying
// the service name because the command that puts it right names the service.
// They are separate types rather than one message because a caller — the
// doctor's --fix, above all — has to tell "there is no such service" from
// "it is there and disabled" without reading the sentence.
type (
	noAgentServiceError     struct{ name string }
	serviceDisabledError    struct{ name string }
	mayNotStartServiceError struct{ name string }
)

func (e noAgentServiceError) Error() string {
	return fmt.Sprintf("this system has no %s service, so there is no agent to reach; "+
		"an administrator can add it with: Add-WindowsCapability -Online -Name OpenSSH.Client~~~~0.0.1.0", e.name)
}

func (e serviceDisabledError) Error() string {
	return fmt.Sprintf("the %s service is disabled, so nothing can start it from here; "+
		"an administrator can enable it with: Set-Service %s -StartupType Automatic", e.name, e.name)
}

func (e mayNotStartServiceError) Error() string {
	return fmt.Sprintf("this account may not start the %s service; "+
		"an administrator can start it with: Start-Service %s", e.name, e.name)
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
		return noAgentServiceError{name: name}
	case errors.Is(err, windows.ERROR_SERVICE_DISABLED):
		return serviceDisabledError{name: name}
	case errors.Is(err, windows.ERROR_ACCESS_DENIED):
		return mayNotStartServiceError{name: name}
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
