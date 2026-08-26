//go:build windows

package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"golang.org/x/sys/windows"
)

// F55: a service that is not installed cannot be enabled, and what comes back
// says so with the command that adds it — never the sentence about privileges,
// which would send somebody to open an administrator's session to fix a thing
// no session can fix.
func TestAServiceThatIsNotThereCannotBeEnabled(t *testing.T) {
	err := systemService{Name: noSuchService}.enable(t.Context())

	require.Error(t, err)
	assert.Contains(t, err.Error(), noSuchService, "the message names which service it was")
	assert.Contains(t, err.Error(), "Add-WindowsCapability",
		"a service that is not there is added, not enabled")
	assert.NotContains(t, err.Error(), "administrator session",
		"nothing here is about privileges")
}

// A refusal to enable is a different sentence from a refusal to start, because
// what puts each right is a different thing: one is a command an administrator
// runs against the service, the other is this same command run again from a
// session that has the privileges doctor is meant to be given.
func TestARefusalToEnableNamesRunningItAgainWithPrivileges(t *testing.T) {
	svc := systemService{Name: "ssh-agent"}

	enabling := svc.explainEnabling(windows.ERROR_ACCESS_DENIED)
	require.Error(t, enabling)
	assert.Contains(t, enabling.Error(), "sshakku doctor --fix",
		"what puts it right is this command, run again with what it needs")
	assert.Contains(t, enabling.Error(), "administrator",
		"and the session it has to be run from")

	starting := svc.explain(windows.ERROR_ACCESS_DENIED)
	require.Error(t, starting)
	assert.Contains(t, starting.Error(), "Start-Service ssh-agent",
		"the start path keeps the sentence it already promised")
}

// Rule 28: a caller who has stopped waiting is not served, and this one writes
// — so the check is worth more here than anywhere else in this file.
func TestAServiceIsNotEnabledForACallerWhoHasGoneAway(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	require.ErrorIs(t, systemService{Name: noSuchService}.enable(ctx), context.Canceled)
}

// Enabling asks for the one right that does it, and that right is where an
// ordinary account is stopped: the service's own security descriptor grants
// changing its configuration to administrators alone. The refusal arrives at
// the opening of the handle, before there is anything to change, which is what
// this measures — the same account reads the configuration in the test beside
// this one, so what is being shown is the boundary and not a service out of
// reach.
//
// It asks for the handle and does nothing with it, deliberately. Calling enable
// here would be a test that writes to the machine it runs on the day some
// machine answers this differently, and a suite that can reconfigure a system
// service by being run on the wrong host is not one anybody should have to
// trust.
func TestAnOrdinaryAccountMayNotOpenTheAgentsServiceToChangeIt(t *testing.T) {
	if elevated(t) {
		t.Skip("this session has the privileges; the refusal being measured cannot happen here")
	}
	svc := systemService{}

	err := svc.withServiceHandle(windows.SERVICE_CHANGE_CONFIG,
		func(windows.Handle) error { return nil }, svc.explainEnabling)

	require.Error(t, err, "changing a service's configuration is an administrator's")
	assert.Contains(t, err.Error(), "sshakku doctor --fix",
		"and the refusal says what to do about it")
}

// elevated reports whether this process is running with the privileges an
// administrator's session has. It is asked only to decide whether a refusal can
// be measured at all, never to decide what the product does: what an account
// may do is settled by asking the service manager, which is the only answer
// that cannot disagree with the attempt.
func elevated(t *testing.T) bool {
	t.Helper()
	token := windows.GetCurrentProcessToken()
	return token.IsElevated()
}
