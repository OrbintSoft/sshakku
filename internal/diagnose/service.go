package diagnose

import (
	"fmt"

	"github.com/OrbintSoft/sshakku/internal/agent"
)

// serviceLine says what the agent's service is doing, in the two halves that
// have to be said together: whether it is serving at this moment, and whether
// anything may start it if it is not.
//
// Either half alone describes a different machine. A service running today says
// nothing about whether the next login will find one, and a service that is
// stopped may be one the next session starts or one that nothing on the machine
// can start — which is the whole difference between an inconvenience and
// something only an administrator can undo.
//
// Where it could not be read at all, what the service manager refused with is
// carried whole rather than reduced to a word: it is already a sentence naming
// what would put it right, and nothing is claimed about a service nobody
// managed to ask about.
func serviceLine(r agent.ServiceReading) string {
	if r.Err != nil {
		return fmt.Sprintf("could not be read — %v", r.Err)
	}
	running := "not running"
	if r.Running {
		running = "running"
	}
	return running + ", " + r.Start.String()
}
