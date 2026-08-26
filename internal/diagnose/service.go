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

// AgentServiceDisabled reports that the agent's service is one nothing on the
// machine may start, which is the one thing in this report that --fix repairs
// by writing outside this account. It is exported so the caller that repairs
// acts on the same reading the report described, rather than going and taking a
// second one that could disagree with what the user was just shown.
func (r Report) AgentServiceDisabled() bool { return serviceIsDisabled(r) }

// serviceIsDisabled reports that the agent's service is one nothing on the
// machine may start. It is the state the rest of the report has to be told
// about, because every other answer it gives assumes a session can put things
// right by opening.
func serviceIsDisabled(r Report) bool {
	return r.AgentService.ServedByAService() && r.AgentService.Start == agent.ServiceStartDisabled
}

// disabledServiceAdvice says what is wrong and what puts it right, in one
// sentence used everywhere the report says either.
//
// One sentence rather than two, because the command in it is the kind of thing
// that drifts: a finding and a recommendation naming different commands for the
// same state is how a reader comes to trust neither.
func disabledServiceAdvice(r Report) string {
	return fmt.Sprintf("the %s service is disabled: nothing on this machine may start it, "+
		"and no new login shell will change that — sshakku doctor --fix, run from an "+
		"administrator session, enables it and starts it", r.AgentService.Name)
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
