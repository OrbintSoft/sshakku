package diagnose

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/agent/inspect"
)

// errProcessesCannotBeEnumeratedHere is the failure this test hands its seam, standing for a real one the
// code under test cannot be made to produce on demand.
var errProcessesCannotBeEnumeratedHere = errors.New("processes cannot be enumerated here")

// The two writings of an endpoint on a system that has two, as F50 hands them
// to a shell of that system's own and to one emulating POSIX.
const (
	pipeNative = `\\.\pipe\openssh-ssh-agent`
	pipePosix  = "//./pipe/openssh-ssh-agent"
)

// F13, F50: a session holding either writing of the endpoint is holding ours,
// and a report that knew only one of them would tell the user their shell is
// pointed at somebody else's agent — sending them to fix what is already right.
func TestEitherWritingOfTheEndpointIsThisSessionsOwn(t *testing.T) {
	for _, writing := range []string{pipeNative, pipePosix} {
		t.Run(writing, func(t *testing.T) {
			r := Gather(t.Context(), Inputs{
				FixedSock:      pipeNative,
				FixedSockPosix: pipePosix,
				EnvSock:        writing,
			}, fakeSource{}, fakeProber{up: map[string]bool{pipeNative: true, pipePosix: true}},
				nil, nil, nil, nil)

			assert.Falsef(t, hasFinding(r, "not our fixed socket"),
				"this session is pointed at our own endpoint: %v", r.Findings)
		})
	}
}

// F13, F51: whether an agent is answering is asked of the endpoint, not worked
// out from a process list. Where the agent is a service there is no process to
// find, and a report that counted processes would tell the user no agent is
// answering while `ssh-add -l` answers in the same window.
func TestAnAgentAnsweringWithNoProcessToShowForItIsStillAnAgent(t *testing.T) {
	r := Gather(t.Context(), Inputs{
		FixedSock:      pipeNative,
		FixedSockPosix: pipePosix,
		EnvSock:        pipePosix,
	}, fakeSource{err: errProcessesCannotBeEnumeratedHere},
		fakeProber{up: map[string]bool{pipeNative: true, pipePosix: true}},
		nil, nil, nil, nil)

	require.True(t, r.FixedReachable, "the endpoint was asked directly")
	assert.Equal(t, StateOursHealthy, r.State, "something answers where sessions are pointed")
	assert.Falsef(t, hasFinding(r, "no ssh-agent is answering"),
		"an agent that answers must not be reported as none: %v", r.Findings)
	assert.Truef(t, hasFinding(r, "could not enumerate processes"),
		"what could not be read is still said, since the report is partial: %v", r.Findings)
}

// F52, F53: a lifetime kept by the sessions rather than by the agent is worth
// saying out loud where somebody looks afterwards, and not only in the log at
// the moment the key was added — what a user gets is not what they asked for,
// but a key removed at the next login rather than at its deadline. Where the
// agent does hold the lifetime that was asked for, there is nothing to say and
// nothing is said.
func TestALifetimeTheAgentCannotHoldIsReported(t *testing.T) {
	t.Run("configured and kept by the sessions", func(t *testing.T) {
		r := Gather(t.Context(), Inputs{FixedSock: fixed, LifetimeKeptBySessions: true},
			fakeSource{}, fakeProber{}, nil, nil, nil, nil)

		assert.Truef(t, hasFinding(r, "holds none"),
			"the report names what the configured lifetime is worth here: %v", r.Findings)
		assert.Truef(t, hasFinding(r, "taken out of the agent as the next session opens"),
			"and says what happens instead, which is the part a user can plan around: %v", r.Findings)
	})

	t.Run("honoured, so unremarkable", func(t *testing.T) {
		r := Gather(t.Context(), Inputs{FixedSock: fixed},
			fakeSource{}, fakeProber{}, nil, nil, nil, nil)

		assert.Falsef(t, hasFinding(r, "holds none"),
			"an agent that keeps its side of the bargain is not worth a finding: %v", r.Findings)
	})
}

// The endpoint answering never overrides what the process list did say. Another
// account's agent on that socket is that account's, and an elevated caller
// reaching it does not make it this session's — which is the state this report
// is about.
func TestAnEndpointAnsweringDoesNotClaimAnotherAccountsAgent(t *testing.T) {
	r := Gather(t.Context(), Inputs{FixedSock: fixed, LegacyDir: legacy, OurUID: 0},
		fakeSource{procs: []inspect.AgentProc{{PID: 100, UID: 1000, Socket: fixed}}},
		fakeProber{up: map[string]bool{fixed: true}}, nil, nil, nil, nil)

	assert.Equal(t, StateClean, r.State,
		"a process list that named somebody else's agent is an answer, and it is not ours")
}

// proberThatKnowsWhoIsHolding is a prober of the richer kind: one that can tell
// an endpoint nobody is on from one somebody else is holding. Only a system
// whose endpoints are names anything may claim has such a prober.
type proberThatKnowsWhoIsHolding struct {
	fakeProber

	held map[string]string
}

func (f proberThatKnowsWhoIsHolding) ReadEndpoint(ctx context.Context, endpoint string) (bool, string) {
	return f.Reachable(ctx, endpoint), f.held[endpoint]
}

// F58: an endpoint held by somebody whose agent it could not be is one SSHakku
// refuses to speak on — and a refusal nobody is told about looks exactly like a
// machine with no agent on it. The report names who is holding it, and does not
// send the reader to open a login shell, which cannot take a name something
// else is already holding.
func TestAnEndpointHeldBySomebodyElseIsSaidSoRatherThanLeftQuiet(t *testing.T) {
	const holder = `NT AUTHORITY\LOCAL SERVICE (process 4321)`

	r := Gather(t.Context(), Inputs{
		FixedSock:      pipeNative,
		FixedSockPosix: pipePosix,
		EnvSock:        pipePosix,
	}, fakeSource{},
		proberThatKnowsWhoIsHolding{held: map[string]string{pipeNative: holder}},
		nil, nil, nil, nil)

	assert.Truef(t, hasFinding(r, holder),
		"the account holding the endpoint, and what is holding it, are named: %v", r.Findings)
	assert.NotContains(t, recommend(r), "login shell",
		"a new shell cannot take a name something else is holding, so it must not be offered as the cure")
	assert.Falsef(t, hasFinding(r, "a new login shell will start one"),
		"and it must not be offered as a finding either, leaving two sentences to choose between: %v", r.Findings)
}

// F58: and the naming must be of somebody real. An endpoint nobody strange is
// holding has nothing to say about who holds it, or the sentence would stand on
// every report of every session and mean nothing on any of them.
func TestAnEndpointNobodyStrangeIsHoldingIsNotReportedAsHeld(t *testing.T) {
	r := Gather(t.Context(), Inputs{
		FixedSock:      pipeNative,
		FixedSockPosix: pipePosix,
		EnvSock:        pipePosix,
	}, fakeSource{},
		proberThatKnowsWhoIsHolding{fakeProber: fakeProber{up: map[string]bool{pipeNative: true, pipePosix: true}}},
		nil, nil, nil, nil)

	assert.Falsef(t, hasFinding(r, "is being held by"),
		"nothing is holding this endpoint but the agent answering on it: %v", r.Findings)
}
