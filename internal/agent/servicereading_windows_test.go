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
