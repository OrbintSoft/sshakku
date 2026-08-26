package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/agent"
	"github.com/OrbintSoft/sshakku/internal/diagnose"
)

// errMayNotEnableTheService is what the service manager refuses an unprivileged
// enable with. It is spelled out here rather than borrowed from the agent
// package because what this test is about is that the sentence reaches the user
// intact, whatever it says.
var errMayNotEnableTheService = errors.New("this session may not enable the ssh-agent service; " +
	"run sshakku doctor --fix again from an administrator session, which is what it is for")

// enablerSpy stands in for the act of enabling the service, recording whether
// it was asked for at all. What it replaces is the write to the machine's
// service manager, not the decision: whether anything is enabled, and on the
// strength of what, is what these cases judge.
type enablerSpy struct {
	err   error
	calls int
}

func (e *enablerSpy) enable(context.Context) error {
	e.calls++
	return e.err
}

// serviceReport is a machine whose agent is served by a service in the given
// state. Nothing else about it matters here: what --fix does about a service is
// decided by the reading the report already holds.
func serviceReport(start agent.ServiceStart) diagnose.Report {
	return diagnose.Report{
		AgentService: agent.ServiceReading{Name: "ssh-agent", Start: start},
	}
}

// serviceDeps builds a doctor reporting on that machine, with the enabling of
// the service standing in for the real write.
func serviceDeps(t *testing.T, report diagnose.Report, enabler *enablerSpy) deps {
	t.Helper()
	tempRuntimeEnv(t)
	d := doctorDeps(report, fakeTokenSource{}, 1000)
	d.enableAgentService = enabler.enable
	return d
}

// TestDoctorFixEnablesTheAgentsService verifies F55: where the agent's service
// is one nothing on the machine may start, `sshakku doctor --fix` is what puts
// it right, and where it may not, it says so having changed nothing.
func TestDoctorFixEnablesTheAgentsService(t *testing.T) {
	t.Run("a disabled service is enabled, and the run says so", func(t *testing.T) {
		enabler := &enablerSpy{}
		d := serviceDeps(t, serviceReport(agent.ServiceStartDisabled), enabler)

		var out, errOut bytes.Buffer
		require.Zerof(t, d.doctor(t.Context(), &out, &errOut, []string{"--fix"}),
			"--fix; stderr=%q", errOut.String())

		assert.Equal(t, 1, enabler.calls, "the one state --fix repairs by writing, repaired once")
		assert.Contains(t, out.String(), "enabled the ssh-agent service",
			"and a run that changed the machine says which change it made")
	})

	t.Run("a service that is merely stopped is left to the lifecycle", func(t *testing.T) {
		enabler := &enablerSpy{}
		d := serviceDeps(t, serviceReport(agent.ServiceStartOnDemand), enabler)

		var out, errOut bytes.Buffer
		require.Zerof(t, d.doctor(t.Context(), &out, &errOut, []string{"--fix"}),
			"--fix; stderr=%q", errOut.String())

		assert.Zero(t, enabler.calls,
			"starting it is what was needed, and rewriting its configuration is not that")
	})

	t.Run("a plain report writes nothing at all", func(t *testing.T) {
		enabler := &enablerSpy{}
		d := serviceDeps(t, serviceReport(agent.ServiceStartDisabled), enabler)

		var out, errOut bytes.Buffer
		require.Zerof(t, d.doctor(t.Context(), &out, &errOut, nil), "stderr=%q", errOut.String())

		assert.Zero(t, enabler.calls, "a report is a look, not an action")
		assert.Contains(t, out.String(), "doctor --fix",
			"what it does instead is name what would put it right")
	})

	t.Run("a session that may not enable it is told, and the run carries on", func(t *testing.T) {
		enabler := &enablerSpy{err: errMayNotEnableTheService}
		d := serviceDeps(t, serviceReport(agent.ServiceStartDisabled), enabler)

		var out, errOut bytes.Buffer
		require.Zerof(t, d.doctor(t.Context(), &out, &errOut, []string{"--fix"}),
			"a refusal to write is not a failure of the whole run; stderr=%q", errOut.String())

		assert.Contains(t, out.String(), "administrator session",
			"the refusal reaches the reader whole, with what to do about it")
		assert.Contains(t, out.String(), "\nafter:\n",
			"and the run goes on to whatever else it could put right")
	})

	t.Run("a system with no service to enable never asks", func(t *testing.T) {
		enabler := &enablerSpy{}
		d := serviceDeps(t, diagnose.Report{}, enabler)

		var out, errOut bytes.Buffer
		require.Zerof(t, d.doctor(t.Context(), &out, &errOut, []string{"--fix"}),
			"--fix; stderr=%q", errOut.String())

		assert.Zero(t, enabler.calls, "there is nothing here that could be disabled")
		assert.NotContains(t, strings.ToLower(out.String()), "agent service:",
			"and nothing is said about a service this system has not got")
	})
}
