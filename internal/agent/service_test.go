package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The failures these tests hand their seams. Each stands for a real one the
// code under test cannot be made to produce on demand.
var (
	errAccessIsDenied       = errors.New("access is denied")
	errTheSshAgentServiceIs = errors.New("the ssh-agent service is disabled; an administrator can enable it")
)

// scriptedService answers with the states it was given, one per question, and
// repeats the last one for as long as it is asked. It records how many times it
// was asked to start, which is the difference between a service driven up once
// and one asked over and over.
type scriptedService struct {
	states   []serviceState
	stateErr error
	startErr error
	starts   int
}

func (s *scriptedService) State(context.Context) (serviceState, error) {
	if s.stateErr != nil {
		return serviceSomethingElse, s.stateErr
	}
	state := s.states[0]
	if len(s.states) > 1 {
		s.states = s.states[1:]
	}
	return state, nil
}

func (s *scriptedService) Start(context.Context) error {
	s.starts++
	return s.startErr
}

// F51: a session that needs the agent gets a working one. A service already
// running is the common case and must cost the login nothing — least of all a
// start request nobody needed.
func TestAServiceAlreadyRunningIsLeftAlone(t *testing.T) {
	svc := &scriptedService{states: []serviceState{serviceRunning}}

	started, err := ensureServiceRunning(t.Context(), svc, time.Second)

	require.NoError(t, err)
	assert.False(t, started, "nobody started it: it was already up")
	assert.Zero(t, svc.starts, "a running service is not asked to start")
}

// F51: where the service is not running, SSHakku starts it, and the session
// carries on with an agent rather than an error.
func TestAStoppedServiceIsStarted(t *testing.T) {
	svc := &scriptedService{states: []serviceState{serviceStopped, serviceRunning}}

	started, err := ensureServiceRunning(t.Context(), svc, time.Second)

	require.NoError(t, err)
	assert.True(t, started, "this call is what started it")
	assert.Equal(t, 1, svc.starts, "started once, not once per question")
}

// Two logins arriving together must cost one start, not two: a service already
// coming up is waited for, and the shell that waited did not start it.
func TestAServiceComingUpIsWaitedForRatherThanStartedAgain(t *testing.T) {
	svc := &scriptedService{states: []serviceState{serviceStarting, serviceStarting, serviceRunning}}

	started, err := ensureServiceRunning(t.Context(), svc, 2*time.Second)

	require.NoError(t, err)
	assert.False(t, started, "somebody else's start is not ours to claim")
	assert.Zero(t, svc.starts, "a service on its way up is not asked again")
}

// F51: what cannot be started is reported in the words the system used, since
// those are what name the command to put it right. The refusal is passed up
// whole rather than turned into a failure of our own.
func TestAStartThatIsRefusedIsReportedAsItWasRefused(t *testing.T) {
	refused := errTheSshAgentServiceIs
	svc := &scriptedService{states: []serviceState{serviceStopped}, startErr: refused}

	started, err := ensureServiceRunning(t.Context(), svc, time.Second)

	require.ErrorIs(t, err, refused, "the sentence a person can act on is the one that arrives")
	assert.False(t, started, "a refused start started nothing")
}

// A service that is started and stops again is answered with that fact. Asking
// twice would not change what is refusing it, and a login must not spend its
// time in a loop that cannot end well.
func TestAServiceThatWillNotStayUpIsNotStartedOverAndOver(t *testing.T) {
	svc := &scriptedService{states: []serviceState{serviceStopped, serviceStopped}}

	started, err := ensureServiceRunning(t.Context(), svc, 2*time.Second)

	require.ErrorIs(t, err, errServiceWillNotStay)
	assert.True(t, started, "it was started, which is why the second look is worth reporting")
	assert.Equal(t, 1, svc.starts, "started once, and then answered rather than retried")
}

// F21, and rule 28: a service that never comes up ends the wait at the bound it
// was given instead of holding the shell open indefinitely.
func TestWaitingForAServiceEndsAtTheBoundItWasGiven(t *testing.T) {
	svc := &scriptedService{states: []serviceState{serviceStarting}}

	start := time.Now()
	started, err := ensureServiceRunning(t.Context(), svc, 250*time.Millisecond)
	elapsed := time.Since(start)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.False(t, started, "waiting is not starting")
	assert.Less(t, elapsed, 3*time.Second, "the bound is what ended it")
}

// A service manager that will not say what the service is doing is not
// answered with a guess: nothing here knows whether starting one would help.
func TestAServiceManagerThatWillNotAnswerIsReported(t *testing.T) {
	refused := errAccessIsDenied
	svc := &scriptedService{states: []serviceState{serviceStopped}, stateErr: refused}

	started, err := ensureServiceRunning(t.Context(), svc, time.Second)

	require.ErrorIs(t, err, refused)
	assert.False(t, started)
	assert.Zero(t, svc.starts, "nothing is started on the strength of an unanswered question")
}
