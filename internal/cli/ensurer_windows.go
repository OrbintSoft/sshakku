//go:build windows

package cli

import (
	"github.com/OrbintSoft/sshakku/internal/agent"
	"github.com/OrbintSoft/sshakku/internal/agent/reach"
	"github.com/OrbintSoft/sshakku/internal/paths"
)

// platformEndpoint is where sessions on this system are pointed: the endpoint
// the system's own agent service serves. Nothing in the resolved layout names
// it — a pipe is not a path — so the layout is not consulted.
func platformEndpoint(paths.Layout) agent.Endpoint { return agent.SystemEndpoint() }

// platformProber is how an agent is asked whether it answers here.
func platformProber() agent.Prober { return reach.PipeProber{} }

// platformEnsurer is the agent lifecycle this system has: a service on an
// endpoint of the system's own, asked whether it answers and started when it
// is not running. The prober takes the waiting policy every probe here takes;
// the service and the endpoint are the system's own and are filled in there.
func platformEnsurer() agentEnsurer {
	return agent.ServiceLifecycle(reach.PipeProber{})
}
