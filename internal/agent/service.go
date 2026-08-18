package agent

import (
	"context"
	"errors"
	"time"
)

// serviceState is what a system's service manager says about the agent's
// service. Only the three states that decide what to do next are told apart;
// everything else — pausing, stopping, whatever a system has of its own — is
// somethingElse, which is waited on rather than acted on, because acting on a
// service in the middle of a transition is how a start request gets refused.
type serviceState int

const (
	serviceSomethingElse serviceState = iota
	serviceStopped
	serviceStarting
	serviceRunning
)

// serviceControl is the part of a system's service manager this package needs:
// what the agent's service is doing, and a request to start it. Which service
// that is belongs to the implementation, since it is the one thing here that
// differs by system; an error from Start is already a sentence the person in
// front of the shell can act on.
type serviceControl interface {
	State(ctx context.Context) (serviceState, error)
	Start(ctx context.Context) error
}

// serviceStartPoll is how often the service is asked again while it comes up.
// A service start is normally over in well under a second, and this is short
// enough not to add a noticeable pause to a login that had to wait for one.
const serviceStartPoll = 100 * time.Millisecond

// errServiceWillNotStay is what a service that was started and then found
// stopped again reports. It is not a refusal — starting was allowed — so it is
// its own answer rather than a start error repeated.
var errServiceWillNotStay = errors.New("the agent's service was started and is not running")

// ensureServiceRunning drives the agent's service to running, waiting up to
// within for it to come up, and reports whether this call is what started it.
//
// A service already running is left alone and reported as nobody's doing. One
// that is stopped is started, once: a service that comes back stopped is
// answered with that fact rather than started again in a loop, since something
// is refusing it that asking twice will not change. A service in the middle of
// something — coming up under another login, going down — is waited on, which
// is what makes two shells starting at the same moment cost one start.
func ensureServiceRunning(ctx context.Context, svc serviceControl, within time.Duration) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, within)
	defer cancel()

	started, asked := false, false
	for {
		state, err := svc.State(ctx)
		if err != nil {
			return started, err
		}
		switch {
		case state == serviceRunning:
			return started, nil
		case state == serviceStopped && asked:
			return started, errServiceWillNotStay
		case state == serviceStopped:
			if err := svc.Start(ctx); err != nil {
				return false, err
			}
			started, asked = true, true
		}
		if err := waitBeforeAskingAgain(ctx); err != nil {
			return started, err
		}
	}
}

// waitBeforeAskingAgain pauses for one poll interval, or reports that the
// caller's context ended first — a deadline reached, or somebody who stopped
// waiting for the shell this is holding up.
func waitBeforeAskingAgain(ctx context.Context) error {
	timer := time.NewTimer(serviceStartPoll)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
