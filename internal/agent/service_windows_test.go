//go:build windows

package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"golang.org/x/sys/windows"
)

// errSomethingTheServiceManagerHas is the failure this test hands its seam, standing for a real one the
// code under test cannot be made to produce on demand.
var errSomethingTheServiceManagerHas = errors.New("something the service manager has not said before")

// noSuchService names a service no system has, so the refusals below are
// exercised for real rather than against whatever this machine is running.
const noSuchService = "sshakku-no-such-service"

// F51: what cannot be started is answered with the command that puts it right.
// A number from the service manager names nothing anybody can act on, so each
// of the three refusals that stand between an account and its agent is turned
// into the sentence that says what to do.
func TestARefusalNamesTheCommandThatPutsItRight(t *testing.T) {
	svc := systemService{Name: "ssh-agent"}

	t.Run("disabled", func(t *testing.T) {
		err := svc.explain(windows.ERROR_SERVICE_DISABLED)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Set-Service ssh-agent -StartupType Automatic",
			"a disabled service is enabled by a command, and the message is where it is named")
	})
	t.Run("refused to this account", func(t *testing.T) {
		err := svc.explain(windows.ERROR_ACCESS_DENIED)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Start-Service ssh-agent",
			"an account that may not start it is told who can, and with what")
	})
	t.Run("not installed at all", func(t *testing.T) {
		err := svc.explain(windows.ERROR_SERVICE_DOES_NOT_EXIST)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Add-WindowsCapability",
			"a service that is not there is added, not started")
	})
	t.Run("anything else", func(t *testing.T) {
		odd := errSomethingTheServiceManagerHas
		require.ErrorIs(t, svc.explain(odd), odd,
			"what is not one of the three arrives whole rather than reworded")
	})
}

// A service somebody else started between the question and the request is the
// outcome that was wanted, and reporting it as a failure would turn two shells
// opening at once into an error in one of them.
func TestAServiceAlreadyRunningIsNotAFailedStart(t *testing.T) {
	assert.NoError(t, systemService{}.explain(windows.ERROR_SERVICE_ALREADY_RUNNING),
		"it is running, which is all that was asked for")
}

// F51: the refusals are what the service manager really answers, not what this
// package guesses it would. A service that cannot exist is the one case that
// can be asked for on any machine, elevated or not.
func TestAServiceThatIsNotThereIsReportedAsSuch(t *testing.T) {
	svc := systemService{Name: noSuchService}

	t.Run("asking what it is doing", func(t *testing.T) {
		state, err := svc.State(t.Context())
		require.Error(t, err, "a service that is not installed cannot be doing anything")
		assert.Contains(t, err.Error(), noSuchService, "the message names which service it was")
		assert.Equal(t, serviceSomethingElse, state, "no state is claimed alongside the refusal")
	})
	t.Run("asking it to start", func(t *testing.T) {
		err := svc.Start(t.Context())
		require.Error(t, err)
		assert.Contains(t, err.Error(), noSuchService, "the message names which service it was")
	})
}

// The agent's service is one this system's service manager can be asked about
// by an ordinary account: opening it asks for reading its state and starting
// it, and nothing more, which is exactly what such an account is granted.
func TestThisSystemAnswersAboutTheAgentsOwnService(t *testing.T) {
	state, err := systemService{}.State(t.Context())

	require.NoError(t, err, "an ordinary account may ask what the agent's service is doing")
	assert.Contains(t,
		[]serviceState{serviceStopped, serviceStarting, serviceRunning, serviceSomethingElse},
		state, "whatever it is doing, it is one of the states the lifecycle reads")
}

// Rule 28: a caller who has stopped waiting is not served. Neither question is
// put to the service manager once the context is done.
func TestACallerWhoHasGoneAwayIsNotServed(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := systemService{}.State(ctx)
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, systemService{}.Start(ctx), context.Canceled)
}

// F51: the states the lifecycle acts on, read off the service manager's own
// numbers. Running is adopted, stopped is started, starting is waited for, and
// everything else — pausing, stopping, a state this system adds in a later
// release — is waited on rather than acted on.
//
// The last of those is the one worth pinning. A state nobody anticipated must
// not fall through to "stopped", because what a session does about stopped is
// start a second agent, and doing that to a service that is merely on its way
// somewhere is how a machine ends up with two.
func TestTheServiceStatesThisLifecycleActsOn(t *testing.T) {
	assert.Equal(t, serviceRunning, stateOf(windows.SERVICE_RUNNING))
	assert.Equal(t, serviceStopped, stateOf(windows.SERVICE_STOPPED))
	assert.Equal(t, serviceStarting, stateOf(windows.SERVICE_START_PENDING))

	for _, midway := range []uint32{
		windows.SERVICE_STOP_PENDING,
		windows.SERVICE_CONTINUE_PENDING,
		windows.SERVICE_PAUSE_PENDING,
		windows.SERVICE_PAUSED,
		// A number this system does not use today, and one it might tomorrow.
		0, 9999,
	} {
		assert.Equalf(t, serviceSomethingElse, stateOf(midway),
			"a state the lifecycle has no move for is waited on, not read as stopped (%d)", midway)
	}
}

// A service name this system cannot even be asked about is reported as that,
// naming the service. The name is configuration, which is somewhere this
// program does not choose what it is handed, and the answer has to be a
// refusal rather than a question asked about some other service.
func TestAServiceNameThisSystemCannotBeAskedAboutIsRefusedByName(t *testing.T) {
	svc := systemService{Name: "ssh\x00agent"}

	_, err := svc.State(t.Context())

	require.Error(t, err, "a name that cannot be looked up is not a service to report on")
	assert.Contains(t, err.Error(), "ssh", "and the refusal names what was asked about")
}

// withServiceHandlesOpenedFor points every handle this package opens at the
// service it names, opened for the rights given here instead of the ones the
// caller asked for.
//
// What is then turned down is turned down by this system's own service manager:
// a handle carries the rights it was opened with, and a call those rights do not
// cover is refused before it does anything. That is what puts these refusals
// within reach of any machine, rather than only of one arranged to produce them
// — and what makes them the service manager's answers instead of a fixture's.
func withServiceHandlesOpenedFor(t *testing.T, granted uint32) {
	t.Helper()
	restore := openServiceHandle
	t.Cleanup(func() { openServiceHandle = restore })
	openServiceHandle = func(manager windows.Handle, name *uint16, _ uint32) (windows.Handle, error) {
		return windows.OpenService(manager, name, granted)
	}
}

// startRequest is what a request to start a service was made with, so a test
// can ask about the request instead of about the machine.
type startRequest struct {
	calls      int
	handle     windows.Handle
	numArgs    uint32
	argVectors **uint16
}

// withStartServiceAnswering puts a fixed answer in the place of the request that
// starts a service, and reports back what the request was made with.
func withStartServiceAnswering(t *testing.T, answer error) *startRequest {
	t.Helper()
	restore := startService
	t.Cleanup(func() { startService = restore })
	made := &startRequest{}
	startService = func(handle windows.Handle, numArgs uint32, argVectors **uint16) error {
		made.calls++
		made.handle, made.numArgs, made.argVectors = handle, numArgs, argVectors
		return answer
	}
	return made
}

// F51: a service whose state cannot be read is reported as that, and never as a
// state somebody gave.
//
// The handle is opened for reading the configuration — a right this account
// has — and then asked what the service is doing, which that right does not
// cover, so the service manager refuses the question rather than the handle.
// Carrying on instead would hand back whatever an unfilled record happens to
// read as, with no error beside it, and nothing would tell a caller that apart
// from an answer.
func TestAServiceWhoseStateCannotBeReadIsNotReportedAsAState(t *testing.T) {
	withServiceHandlesOpenedFor(t, windows.SERVICE_QUERY_CONFIG)

	state, err := systemService{}.State(t.Context())

	require.Error(t, err, "the question was refused, so there is no state to report")
	assert.Contains(t, err.Error(), agentServiceName, "and the refusal names which service it was about")
	assert.Equal(t, serviceSomethingElse, state, "no state is claimed alongside a refusal")
}

// F51: the request to start is really made through the handle opened for it,
// and a refusal of the request — rather than of the handle — is still turned
// into the sentence naming who can start the service and how.
//
// The handle here carries reading the service's state and nothing else, so the
// service manager turns the start down. Nothing is started, and nothing could
// be: the right to do it was never on the handle.
func TestAStartTheServiceManagerItselfRefusesStillNamesWhoCanDoIt(t *testing.T) {
	withServiceHandlesOpenedFor(t, windows.SERVICE_QUERY_STATUS)

	err := systemService{}.Start(t.Context())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Start-Service "+agentServiceName,
		"an account that may not start it is told who can, and with what")
}

// F51: a service somebody else started between the question and the request is
// the outcome that was wanted, and it has to reach the caller as one.
//
// Two shells opening at once is the ordinary way this happens. The answer is put
// in place here rather than provoked, because the only machine that produces it
// is one where something is being started — and starting things on the system
// under test is not how this gets to be sure.
func TestAServiceStartedBySomebodyElseFirstIsNotAFailedStart(t *testing.T) {
	withServiceHandlesOpenedFor(t, windows.SERVICE_QUERY_STATUS)
	made := withStartServiceAnswering(t, windows.ERROR_SERVICE_ALREADY_RUNNING)

	require.NoError(t, systemService{}.Start(t.Context()),
		"it is running, which is all that was asked for")
	assert.Equal(t, 1, made.calls, "and it was really asked for, not skipped")
}

// A start the service manager grants is reported as one, and what was asked for
// is the plain thing: this service, started the way the machine has it
// configured. Arguments handed to a service at start time are no part of what
// this program offers, and a service that got some would be running as
// something other than what the machine was set up to run.
func TestAStartIsAskedForPlainlyAndItsSuccessReported(t *testing.T) {
	withServiceHandlesOpenedFor(t, windows.SERVICE_QUERY_STATUS)
	made := withStartServiceAnswering(t, nil)

	require.NoError(t, systemService{}.Start(t.Context()))

	assert.Equal(t, 1, made.calls)
	assert.NotZero(t, made.handle, "the request went through the handle that was opened for it")
	assert.Zero(t, made.numArgs, "the service is started as it is configured")
	assert.Nil(t, made.argVectors, "with nothing handed to it from here")
}
