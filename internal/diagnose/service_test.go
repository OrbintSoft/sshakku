package diagnose

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/agent"
	"github.com/OrbintSoft/sshakku/internal/platform"
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

// F13: a count of nothing is a claim to have looked and found nothing. Where
// this system does not list agent processes at all, saying "0" tells a reader
// the opposite of what the report has just told them one line above — that a
// service is running and serving their agent.
func TestASystemThatDoesNotListAgentProcessesIsNotReportedAsHavingNone(t *testing.T) {
	out := formatted(t, Report{
		FixedSock:    `\\.\pipe\openssh-ssh-agent`,
		AgentService: agent.ServiceReading{Name: "ssh-agent", Running: true, Start: agent.ServiceStartAutomatic},
		InspectErr:   platform.Unimplemented("enumerating ssh-agent processes"),
	})

	assert.NotContains(t, out, "ssh-agent processes (0)",
		"nothing was counted, so no count is printed")
	assert.NotContains(t, out, "(none)", "nor is the emptiness of a list nobody read")
	assert.Contains(t, out, "not listed on this system",
		"what a reader gets instead is why there is no list")
	assert.Contains(t, out, "the service above",
		"and where the answer they came for actually is")
}

// The same absence, in the findings: a report is partial when something it
// meant to read could not be read. A list this system was never going to keep
// is not a piece missing from the report — saying so on every single run would
// train a reader to read past the findings.
func TestASystemThatKeepsNoSuchListIsNotAPartialReport(t *testing.T) {
	f := strings.Join(findings(Inputs{}, Report{
		AgentService: agent.ServiceReading{Name: "ssh-agent", Running: true},
		InspectErr:   platform.Unimplemented("enumerating ssh-agent processes"),
	}), "\n")

	assert.NotContains(t, f, "report is partial",
		"nothing is missing from a report about a system built this way")
}

// An enumeration that was meant to work and did not is exactly what that
// finding is for, and it still says so. The two are told apart by what the
// failure is, not by which system it happened on.
func TestAnEnumerationThatFailedIsStillAPartialReport(t *testing.T) {
	f := strings.Join(findings(Inputs{}, Report{
		InspectErr: errors.New("/proc: permission denied"),
	}), "\n")

	assert.Contains(t, f, "could not enumerate processes")
	assert.Contains(t, f, "report is partial")
	assert.Contains(t, f, "/proc: permission denied", "with what actually went wrong")
}

// F55: a disabled service is not started by the next session, so the report
// must not send anybody to open one. The line a reader is most likely to act on
// is the recommendation, and on a machine in this state the one it used to give
// was the one thing that could not work.
func TestADisabledServiceIsNotAnsweredByOpeningANewShell(t *testing.T) {
	disabled := Report{
		State:        StateClean,
		AgentService: agent.ServiceReading{Name: "ssh-agent", Start: agent.ServiceStartDisabled},
	}

	advice := recommend(disabled)

	assert.NotContains(t, advice, "a new login shell starts one",
		"nobody is sent to open a shell that will start nothing")
	assert.Contains(t, advice, "no new login shell will change that",
		"they are told the opposite, since that is what they would otherwise try")
	assert.Contains(t, advice, "doctor --fix", "what puts it right is named")
	assert.Contains(t, advice, "administrator", "and what it takes to run it")
}

// The same state, in the findings, since that is the other place a reader
// looks. It says what is wrong before it says what to do about it: a reader who
// disagrees with the advice still needs the fact.
func TestADisabledServiceIsAFindingOfItsOwn(t *testing.T) {
	f := strings.Join(findings(Inputs{}, Report{
		AgentService: agent.ServiceReading{Name: "ssh-agent", Start: agent.ServiceStartDisabled},
	}), "\n")

	assert.Contains(t, f, "ssh-agent", "the service is named")
	assert.Contains(t, f, "disabled")
	assert.Contains(t, f, "doctor --fix")
}

// A service that is merely stopped is exactly what the next session starts, so
// the advice that was already there is the right advice and stays. Only the
// disabled case was ever wrong, and a change that swept up both would take a
// true sentence away with the false one.
func TestAStoppedServiceIsStillAnsweredByOpeningANewShell(t *testing.T) {
	stopped := Report{
		State:        StateClean,
		AgentService: agent.ServiceReading{Name: "ssh-agent", Start: agent.ServiceStartOnDemand},
	}

	assert.Contains(t, recommend(stopped), "login shell",
		"a session starts this one on its way in, which is the whole of the repair")
}

// A system whose agent is a process on a socket has no service, and the report
// says nothing at all about one. An empty section would have every Linux and
// macOS report carrying a heading for a thing that platform has not got.
func TestASystemWithNoServiceHasNoSectionForOne(t *testing.T) {
	out := formatted(t, Report{FixedSock: "/run/user/1000/sshakku/agent.sock"})

	assert.NotContains(t, out, "agent service:",
		"there is no service here, so there is nothing to head a section with")
}
