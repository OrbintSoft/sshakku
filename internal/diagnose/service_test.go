package diagnose

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/agent"
)

// formatted renders a report and hands back what it printed. Every check below
// reads the report a user reads, rather than the structure behind it: what this
// step is about is what the report says.
func formatted(t *testing.T, r Report) string {
	t.Helper()
	var out strings.Builder
	Format(&out, r)
	return out.String()
}

// F55: the report names the service the agent is served from and what it is
// doing. Both halves are said, because either alone is the state of a different
// machine: a service that is running says nothing about whether it will be
// there next time, and one that is disabled is not merely stopped.
func TestTheReportNamesTheServiceAndWhatItIsDoing(t *testing.T) {
	for _, tc := range []struct {
		name    string
		reading agent.ServiceReading
		says    string
	}{
		{
			name:    "running and set to start itself",
			reading: agent.ServiceReading{Name: "ssh-agent", Running: true, Start: agent.ServiceStartAutomatic},
			says:    "running, starts automatically",
		},
		{
			name:    "stopped, and something may start it",
			reading: agent.ServiceReading{Name: "ssh-agent", Start: agent.ServiceStartOnDemand},
			says:    "not running, starts on demand",
		},
		{
			name:    "stopped, and nothing may start it",
			reading: agent.ServiceReading{Name: "ssh-agent", Start: agent.ServiceStartDisabled},
			says:    "not running, disabled",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := formatted(t, Report{AgentService: tc.reading})

			assert.Contains(t, out, "agent service:", "the section is there to be found")
			assert.Contains(t, out, "ssh-agent", "the service is named, since a reader has to go and act on that name")
			assert.Contains(t, out, tc.says)
		})
	}
}

// F41: a report that could not determine something says so and still arrives.
// What the service manager refused with is already a sentence naming what would
// put it right, so it is carried rather than reduced to "unavailable".
func TestAServiceThatCouldNotBeReadIsStillReported(t *testing.T) {
	refused := errors.New("this system has no ssh-agent service, so there is no agent to reach; " +
		"an administrator can add it with: Add-WindowsCapability -Online -Name OpenSSH.Client~~~~0.0.1.0")

	out := formatted(t, Report{AgentService: agent.ServiceReading{Name: "ssh-agent", Err: refused}})

	require.Contains(t, out, "agent service:", "a section that could not be filled is still a section")
	assert.Contains(t, out, "Add-WindowsCapability",
		"what the service manager said is what the reader needs, whole")
	assert.NotContains(t, out, "not running, start type unknown",
		"nothing is claimed about a service nobody could read")
}

// A system whose agent is a process on a socket has no service, and the report
// says nothing at all about one. An empty section would have every Linux and
// macOS report carrying a heading for a thing that platform has not got.
func TestASystemWithNoServiceHasNoSectionForOne(t *testing.T) {
	out := formatted(t, Report{FixedSock: "/run/user/1000/sshakku/agent.sock"})

	assert.NotContains(t, out, "agent service:",
		"there is no service here, so there is nothing to head a section with")
}
