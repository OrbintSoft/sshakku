//go:build windows

package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"golang.org/x/sys/windows"
)

// F55: the report says how the agent's service will be started, which is the
// one thing asking what it is doing never answers — a disabled service reports
// itself stopped, exactly as one waiting to be asked for does. An ordinary
// account may read it: the service's own security descriptor grants reading its
// configuration to interactive accounts and reserves changing it for
// administrators, so this answers from the session a person is sitting in.
func TestTheAgentsServiceSaysHowItWillBeStarted(t *testing.T) {
	reading := ReadAgentService(t.Context())

	require.NoError(t, reading.Err, "an ordinary account may ask how the agent's service starts")
	assert.Equal(t, agentServiceName, reading.Name, "the reading names the service it read")
	assert.True(t, reading.ServedByAService(), "this system serves its agent from a service")
	assert.Contains(t,
		[]ServiceStart{ServiceStartAutomatic, ServiceStartOnDemand, ServiceStartDisabled},
		reading.Start, "however it is set, it is one of the answers this vocabulary has a word for")
}

// F41: the report is a look, not an action. Reading how a service starts must
// not be what starts it — on a machine whose agent service is disabled or
// merely stopped, a diagnostic that started it would repair the very thing it
// was asked to describe, and nobody would ever see the state they ran it for.
func TestReadingTheServiceLeavesItDoingWhatItWasDoing(t *testing.T) {
	before, err := systemService{}.State(t.Context())
	require.NoError(t, err)

	require.NoError(t, ReadAgentService(t.Context()).Err)

	after, err := systemService{}.State(t.Context())
	require.NoError(t, err)
	assert.Equal(t, before, after, "the service is doing afterwards what it was doing before")
}

// A service that is not installed has no start type, and the reading says so
// rather than reporting the zero value as an answer somebody read. What it
// carries instead is the sentence naming what would put it right, which is the
// one the service manager's refusal already produces.
func TestAServiceThatIsNotThereHasNoStartTypeToReport(t *testing.T) {
	reading := systemService{Name: noSuchService}.read(t.Context())

	require.Error(t, reading.Err, "a service that is not installed cannot say how it starts")
	assert.Contains(t, reading.Err.Error(), "Add-WindowsCapability",
		"the refusal carries the command that puts it right")
	assert.Equal(t, ServiceStartUnknown, reading.Start, "no start type is claimed alongside the refusal")
	assert.False(t, reading.Running, "nor is it claimed to be running")
	assert.Equal(t, noSuchService, reading.Name, "the reading still names what was asked about")
}

// The words this system's service manager uses for how a service starts, read
// as the answers a report has a word for. Only one of these can be observed on
// any given machine — whatever that machine happens to be set to — so the
// mapping is asked for directly rather than through the one service this
// account has, which would leave every other answer unchecked until somebody
// met it.
func TestThisSystemsWordsForHowAServiceStartsAreRead(t *testing.T) {
	for _, tc := range []struct {
		raw  uint32
		read ServiceStart
	}{
		{windows.SERVICE_AUTO_START, ServiceStartAutomatic},
		{windows.SERVICE_BOOT_START, ServiceStartAutomatic},
		{windows.SERVICE_SYSTEM_START, ServiceStartAutomatic},
		{windows.SERVICE_DEMAND_START, ServiceStartOnDemand},
		{windows.SERVICE_DISABLED, ServiceStartDisabled},
		{^uint32(0), ServiceStartUnknown},
	} {
		assert.Equal(t, tc.read, startTypeOf(tc.raw), "what this system's %d means", tc.raw)
	}
}

// The handle a question is asked through carries the right to ask it, and each
// caller opens the service for the one right it uses rather than for
// everything. A handle opened to read the service's state cannot read its
// configuration, and this is what says so — the service manager refusing it for
// real, rather than a comment claiming the rights are separate.
func TestAHandleOpenedToReadTheStateCannotReadTheConfiguration(t *testing.T) {
	err := systemService{}.withService(windows.SERVICE_QUERY_STATUS, func(handle windows.Handle) error {
		_, err := serviceStartType(handle)
		return err
	})

	require.Error(t, err, "the right to see what it is doing is not the right to see how it starts")
}

// Rule 28: a caller who has stopped waiting is not served. The reading is two
// questions to the service manager and neither is put once the context is done.
func TestAReadingIsNotTakenForACallerWhoHasGoneAway(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	reading := ReadAgentService(ctx)

	require.ErrorIs(t, reading.Err, context.Canceled)
	assert.Equal(t, agentServiceName, reading.Name,
		"which service was being asked about is known without asking anybody")
}

// Rule 28: how a service is started is read from the machine, and a caller who
// has stopped waiting is not served. This one only reads, but the check is what
// makes the answer come back at all: a report asked for by a shell that has
// since gone is a report nobody will read.
func TestHowAServiceStartsIsNotLookedUpForACallerWhoHasGoneAway(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := systemService{}.startType(ctx)

	require.ErrorIs(t, err, context.Canceled)
}

// F55: a read of the configuration that the service manager refuses is reported
// — naming the service and what was being asked about it — rather than answered
// with a start type nobody gave.
//
// The handle is opened for reading the service's state and nothing else, so the
// refusal happens at the read and not at the handle: this is the case where the
// service was reached and the question still could not be put.
func TestAConfigurationThatCannotBeReadIsNoStartTypeToReport(t *testing.T) {
	withServiceHandlesOpenedFor(t, windows.SERVICE_QUERY_STATUS)

	start, err := systemService{}.startType(t.Context())

	require.Error(t, err, "the read was refused, so there is no start type")
	assert.Contains(t, err.Error(), "asking how the "+agentServiceName+" service is started",
		"the message names the service and the question that was put about it")
	assert.Equal(t, ServiceStartUnknown, start, "and no start type is claimed alongside the refusal")
}

// F41: what the machine was willing to say is kept, even when the rest of the
// reading is refused.
//
// An account may be allowed to see that the service is running and not allowed
// to see how it starts. That account has still been told something, and a report
// that threw it away would leave a reader with less than the machine offered —
// and with no way to tell a question that could not be put from one that was put
// and answered.
//
// The handle is opened for reading the state and nothing else, so the first
// question is answered for real and the second refused for real. What the first
// one answers is whatever this machine's service is doing, which is why it is
// taken once with nothing in the way and then compared.
func TestWhatTheMachineAnsweredBeforeARefusalStaysInTheReading(t *testing.T) {
	answered := ReadAgentService(t.Context())
	require.NoError(t, answered.Err, "this account can take the whole reading when nothing is in the way")

	withServiceHandlesOpenedFor(t, windows.SERVICE_QUERY_STATUS)
	reading := ReadAgentService(t.Context())

	assert.Equal(t, agentServiceName, reading.Name, "which service it was about is still there")
	assert.Equal(t, answered.Running, reading.Running,
		"and so is the half that was answered, unchanged from what it says when nothing is refused")
	require.Error(t, reading.Err, "the refused half is reported")
	assert.Equal(t, ServiceStartUnknown, reading.Start, "with nothing claimed in its place")
}
