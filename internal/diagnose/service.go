package diagnose

import (
	"errors"
	"fmt"

	"github.com/OrbintSoft/sshakku/internal/agent"
	"github.com/OrbintSoft/sshakku/internal/platform"
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
// keepsNoAgentProcessList reports that the system being reported on does not
// list ssh-agent processes at all, as against having been asked and failed.
//
// The two look the same from the report's side — an enumeration that returned
// no answer — and they call for opposite handling: one is a piece the report
// could not get and should say it is missing, the other is the shape of the
// system, where saying it on every single run would train a reader to read past
// the findings.
func keepsNoAgentProcessList(r Report) bool {
	return errors.Is(r.InspectErr, platform.ErrUnimplemented)
}

// processListNote says what stands in place of the list on such a system, and
// where the answer the reader came for actually is.
func processListNote(r Report) string {
	if r.AgentService.ServedByAService() {
		return "not listed on this system — the agent is served by the service above"
	}
	return "not listed on this system"
}

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
