//go:build windows

package agent

import (
	"context"
	"fmt"
	"time"
)

// serviceStartWait bounds how long a login waits for the agent's service to
// come up. A service start is normally over in a fraction of this; what the
// bound is for is the login that would otherwise wait on one that never will.
const serviceStartWait = 10 * time.Second

// ServiceAgent drives the agent on a system where it is a service on a fixed
// endpoint of the system's own, rather than a process this program starts on a
// socket it chose.
//
// Nearly everything the socket lifecycle does has no counterpart here, and
// that is the mechanism rather than work left undone. There is no socket file
// to go stale, no second agent of ours to tell from a foreign one, and nothing
// to adopt: the endpoint is the system's, every session on the machine shares
// it, and the keys reached through it are each account's own because the
// service serves each caller their own. What is left is the question that
// matters — does it answer — and the one repair available: start the service.
type ServiceAgent struct {
	// Prober asks whether an agent answers on the endpoint.
	Prober Prober
	// Service is the agent's service, as this system's service manager knows it.
	Service serviceControl
	// Endpoint is where the agent is reached.
	Endpoint Endpoint
}

// ServiceLifecycle is what drives the agent on this system, given the prober
// to ask with. Which service and which endpoint are this system's own and are
// filled in here; the prober carries a waiting policy, which is the caller's.
func ServiceLifecycle(prober Prober) ServiceAgent {
	return ServiceAgent{Prober: prober, Service: systemService{}, Endpoint: SystemEndpoint()}
}

// EnsureAgent drives the endpoint to a healthy agent and returns it.
//
// The EnsureConfig a socket system needs is not read here: no path of ours is
// bound, so there is no stale socket to clear and no state to record, and the
// start is serialised by the service manager itself rather than by a lock file
// of ours — two logins arriving together cost one start whichever gets there
// first.
func (a ServiceAgent) EnsureAgent(ctx context.Context, _ EnsureConfig, log Logger) (EnsureResult, error) {
	logf := func(level, format string, args ...any) {
		if log != nil {
			_ = log.Log(level, fmt.Sprintf(format, args...))
		}
	}

	if a.Prober.Reachable(ctx, a.Endpoint.Native()) {
		return EnsureResult{Situation: SituationHealthy, Live: a.Endpoint}, nil
	}

	started, err := ensureServiceRunning(ctx, a.Service, serviceStartWait)
	if err != nil {
		return EnsureResult{}, err
	}

	// Running is not answering. Asking again is the whole of the check: a
	// service that has just been started is listening a moment later, and one
	// that was already running and still says nothing is a state nothing here
	// can repair — stopping somebody else's agent is not ours to do.
	if !a.Prober.Reachable(ctx, a.Endpoint.Native()) {
		return EnsureResult{}, fmt.Errorf(
			"the agent's service is running and nothing answers on %s", a.Endpoint.Native())
	}
	if !started {
		return EnsureResult{Situation: SituationHealthy, Live: a.Endpoint}, nil
	}
	logf("INFO", "started the agent's service; it answers on %s", a.Endpoint.Native())
	return EnsureResult{Situation: SituationClean, Live: a.Endpoint}, nil
}
