//go:build windows

package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// scriptedProber answers with the answers it was given, one per question, and
// repeats the last for as long as it is asked. It records what it was asked
// about, since pointing a session at one endpoint while probing another is a
// mistake that would otherwise pass every assertion here.
type scriptedProber struct {
	answers []bool
	asked   []string
}

func (p *scriptedProber) Reachable(_ context.Context, endpoint string) bool {
	p.asked = append(p.asked, endpoint)
	answer := p.answers[0]
	if len(p.answers) > 1 {
		p.answers = p.answers[1:]
	}
	return answer
}

// F50: an agent already answering is what a login wants, and the endpoint it is
// pointed at is the system's own — reached without the service manager being
// troubled at all.
func TestAnAgentThatAnswersIsUsedAsItIs(t *testing.T) {
	prober := &scriptedProber{answers: []bool{true}}
	service := &scriptedService{states: []serviceState{serviceStopped}}
	lifecycle := ServiceAgent{Prober: prober, Service: service, Endpoint: PipeEndpoint(`\\.\pipe\test`)}

	res, err := lifecycle.EnsureAgent(t.Context(), EnsureConfig{}, nil)

	require.NoError(t, err)
	assert.Equal(t, SituationHealthy, res.Situation)
	assert.Equal(t, `\\.\pipe\test`, res.Live.Native(), "the session is pointed at the endpoint that answered")
	assert.Equal(t, []string{`\\.\pipe\test`}, prober.asked, "and that is the endpoint that was asked")
	assert.Zero(t, service.starts, "a service is not started for an agent that is already answering")
}

// F51: silence is answered by starting the service, and the session then gets
// its agent. The endpoint is asked again afterwards, because a service that is
// running is not yet a promise that something answers on it.
func TestASilentEndpointIsAnsweredByStartingTheService(t *testing.T) {
	prober := &scriptedProber{answers: []bool{false, true}}
	service := &scriptedService{states: []serviceState{serviceStopped, serviceRunning}}
	lifecycle := ServiceAgent{Prober: prober, Service: service, Endpoint: PipeEndpoint(`\\.\pipe\test`)}

	res, err := lifecycle.EnsureAgent(t.Context(), EnsureConfig{}, nil)

	require.NoError(t, err)
	assert.Equal(t, SituationClean, res.Situation, "nothing was running, and this call started it")
	assert.Equal(t, `\\.\pipe\test`, res.Live.Native())
	assert.Equal(t, 1, service.starts)
	assert.Len(t, prober.asked, 2, "asked once before starting and once after")
}

// A service somebody else's login started, on an endpoint that then answers, is
// nobody's doing here — and reporting it as ours would put a start in the log
// that never happened.
func TestAServiceSomebodyElseStartedIsNotClaimed(t *testing.T) {
	prober := &scriptedProber{answers: []bool{false, true}}
	service := &scriptedService{states: []serviceState{serviceRunning}}
	lifecycle := ServiceAgent{Prober: prober, Service: service, Endpoint: PipeEndpoint(`\\.\pipe\test`)}

	res, err := lifecycle.EnsureAgent(t.Context(), EnsureConfig{}, nil)

	require.NoError(t, err)
	assert.Equal(t, SituationHealthy, res.Situation, "it was up already; the first probe was simply too early")
	assert.Zero(t, service.starts)
}

// F51: a refusal is what the person in front of the shell has to act on, so it
// arrives exactly as the service manager worded it.
func TestAServiceThatCannotBeStartedIsReportedInItsOwnWords(t *testing.T) {
	refused := errors.New("the ssh-agent service is disabled; an administrator can enable it")
	prober := &scriptedProber{answers: []bool{false}}
	service := &scriptedService{states: []serviceState{serviceStopped}, startErr: refused}
	lifecycle := ServiceAgent{Prober: prober, Service: service, Endpoint: PipeEndpoint(`\\.\pipe\test`)}

	res, err := lifecycle.EnsureAgent(t.Context(), EnsureConfig{}, nil)

	require.ErrorIs(t, err, refused)
	assert.Empty(t, res.Live.Native(),
		"a session is never pointed at an endpoint nothing was shown to answer on")
}

// Running is not answering, and a service that is up while the endpoint stays
// silent is a state nothing here can repair — stopping somebody else's agent is
// not ours to do. It is said plainly rather than papered over with a session
// pointed at silence.
func TestAnEndpointThatStaysSilentIsNotHandedToTheShell(t *testing.T) {
	prober := &scriptedProber{answers: []bool{false, false}}
	service := &scriptedService{states: []serviceState{serviceStopped, serviceRunning}}
	lifecycle := ServiceAgent{Prober: prober, Service: service, Endpoint: PipeEndpoint(`\\.\pipe\test`)}

	res, err := lifecycle.EnsureAgent(t.Context(), EnsureConfig{}, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), `\\.\pipe\test`, "the message names where nothing answered")
	assert.Empty(t, res.Live.Native())
}

// What the product composes for this system is the service lifecycle, pointed
// at this system's own service and endpoint.
func TestTheLifecycleThisSystemGetsIsPointedAtItsOwnAgent(t *testing.T) {
	lifecycle := ServiceLifecycle(&scriptedProber{answers: []bool{true}})

	assert.Equal(t, SystemEndpoint(), lifecycle.Endpoint, "this system's endpoint")
	assert.Equal(t, systemService{}, lifecycle.Service, "and this system's service manager")
}
