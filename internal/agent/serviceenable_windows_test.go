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

// Rule 28 again, at the door this time: EnableAgentService is what the
// diagnostic tool calls, and a caller that has stopped waiting must not have
// the machine changed on its behalf. Nothing is opened and nothing is written.
func TestTheMachineIsNotChangedForACallerWhoHasGoneAway(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	require.ErrorIs(t, EnableAgentService(ctx), context.Canceled,
		"a caller who has gone away does not get a machine-wide change made for them")
}

// A refusal this code has no sentence of its own for is still reported, with
// what the service manager said kept inside it. Answering only the two refusals
// that were anticipated would leave every other one as a bare failure with
// nothing in it to act on, and the service manager has a long list.
func TestARefusalToEnableNobodyAnticipatedStillSaysWhatHappened(t *testing.T) {
	err := systemService{Name: "ssh-agent"}.explainEnabling(errSomethingTheServiceManagerHas)

	require.Error(t, err)
	assert.ErrorIs(t, err, errSomethingTheServiceManagerHas,
		"what the service manager said is kept, not replaced by a sentence of ours")
	assert.Contains(t, err.Error(), "ssh-agent", "and which service it was about")
}

// configWrite is what a service's configuration was asked to be written with.
// A test asks about the call, because a machine inspected afterwards cannot
// tell a field that was left alone from one rewritten to the value it already
// had.
type configWrite struct {
	calls            int
	handle           windows.Handle
	serviceType      uint32
	startType        uint32
	errorControl     uint32
	binaryPathName   *uint16
	loadOrderGroup   *uint16
	tagID            *uint32
	dependencies     *uint16
	serviceStartName *uint16
	password         *uint16
	displayName      *uint16
}

// withChangeServiceConfigAnswering puts a fixed answer in the place of the call
// that writes a service's configuration, and reports back what it was given.
func withChangeServiceConfigAnswering(t *testing.T, answer error) *configWrite {
	t.Helper()
	restore := changeServiceConfig
	t.Cleanup(func() { changeServiceConfig = restore })
	made := &configWrite{}
	changeServiceConfig = func(handle windows.Handle, serviceType, startType, errorControl uint32,
		binaryPathName, loadOrderGroup *uint16, tagID *uint32,
		dependencies, serviceStartName, password, displayName *uint16,
	) error {
		made.calls++
		made.handle = handle
		made.serviceType, made.startType, made.errorControl = serviceType, startType, errorControl
		made.binaryPathName, made.loadOrderGroup, made.tagID = binaryPathName, loadOrderGroup, tagID
		made.dependencies, made.serviceStartName = dependencies, serviceStartName
		made.password, made.displayName = password, displayName
		return answer
	}
	return made
}

// F55: enabling asks for one thing. The service manager is given the start type
// and, for every other field, the word that means leave this as it is — so a
// service enabled here keeps the program it runs, the account it runs as, the
// name it is listed under and everything else it was set to.
//
// It is checked at the call rather than on the machine afterwards, because a
// machine inspected later cannot tell a field left alone from one rewritten to
// the value it already had. The day one of these arguments is something else,
// sshakku doctor --fix is a command that reconfigures a system service on its
// way to enabling it, which is a far larger thing than the report offered to do
// and one nobody would notice until after it had happened.
func TestEnablingChangesHowTheServiceStartsAndNothingElseAboutIt(t *testing.T) {
	withServiceHandlesOpenedFor(t, windows.SERVICE_QUERY_STATUS)
	written := withChangeServiceConfigAnswering(t, nil)

	require.NoError(t, systemService{}.enable(t.Context()))

	require.Equal(t, 1, written.calls, "the configuration is written once")
	assert.NotZero(t, written.handle, "through the handle that was opened to change it")
	assert.Equal(t, uint32(windows.SERVICE_AUTO_START), written.startType,
		"the service is set back to starting by itself, which is what a system nobody has changed has it at")
	assert.Equal(t, uint32(windows.SERVICE_NO_CHANGE), written.serviceType,
		"what kind of service it is stays as it is")
	assert.Equal(t, uint32(windows.SERVICE_NO_CHANGE), written.errorControl,
		"and so does what this system does when it fails to start")
	assert.Nil(t, written.binaryPathName, "the program it runs is not rewritten on the way to enabling it")
	assert.Nil(t, written.serviceStartName, "nor the account it runs as")
	assert.Nil(t, written.password, "nor that account's password")
	assert.Nil(t, written.displayName, "nor the name somebody reads in the services list")
	assert.Nil(t, written.loadOrderGroup, "nor where it comes in the order things are started")
	assert.Nil(t, written.tagID, "nor its place within that")
	assert.Nil(t, written.dependencies, "nor what it waits for")
}

// F55: a refusal met at the write itself, rather than at the handle, is still
// the enabling sentence — and still sends somebody to run this same command
// again with what it needs, instead of to the other command entirely.
//
// The handle carries reading the service's state and nothing else, so the
// service manager turns the write down. Nothing is written, and nothing could
// be: the right to write was never on the handle.
func TestAWriteTheServiceManagerRefusesIsStillTheEnablingSentence(t *testing.T) {
	withServiceHandlesOpenedFor(t, windows.SERVICE_QUERY_STATUS)

	err := systemService{}.enable(t.Context())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "sshakku doctor --fix",
		"what puts it right is this command, run again with what it needs")
	assert.NotContains(t, err.Error(), "Start-Service",
		"a refused write is not a refused start, and does not send anybody to the command for one")
}

// F55: a service that is not there is not written to. The refusal arrives at the
// handle, and what comes after the handle never runs — so a system without the
// service is one this leaves alone entirely, rather than one it attempts a write
// on and is turned down by.
func TestAServiceThatIsNotThereIsNeverWrittenTo(t *testing.T) {
	written := withChangeServiceConfigAnswering(t, nil)

	require.Error(t, systemService{Name: noSuchService}.enable(t.Context()))

	assert.Zero(t, written.calls, "there was nothing to write to, so nothing was written")
}
