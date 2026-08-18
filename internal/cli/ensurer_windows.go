//go:build windows

package cli

import (
	"github.com/OrbintSoft/sshakku/internal/agent"
	"github.com/OrbintSoft/sshakku/internal/agent/reach"
)

// platformEnsurer is the agent lifecycle this system has: a service on an
// endpoint of the system's own, asked whether it answers and started when it
// is not running. The prober takes the waiting policy every probe here takes;
// the service and the endpoint are the system's own and are filled in there.
func platformEnsurer() agentEnsurer {
	return agent.ServiceLifecycle(reach.PipeProber{})
}
