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
