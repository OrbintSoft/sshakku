package diagnose

import "github.com/OrbintSoft/sshakku/internal/agent/inspect"

// State names the agent-lifecycle situation the report observes, following the
// five states the login path resolves (clean, ours-healthy, ours-zombie,
// foreign-healthy, disaster). The doctor only reports the state; the login path
// is what acts on it.
type State int

const (
	// StateUnknown is the zero value, before classification.
	StateUnknown State = iota
	// StateClean — nothing is running; a login starts fresh.
	StateClean
	// StateOursHealthy — our agent answers on the fixed socket.
	StateOursHealthy
	// StateOursZombie — only dead remnants of our agent remain.
	StateOursZombie
	// StateForeignHealthy — a single agent we did not start is answering.
	StateForeignHealthy
	// StateDisaster — several agents answer at once; the situation is mixed.
	StateDisaster
)

func (s State) String() string {
	switch s {
	case StateClean:
		return "A — clean (no agent running)"
	case StateOursHealthy:
		return "B — ours healthy"
	case StateOursZombie:
		return "C — ours zombie (dead remnant)"
	case StateForeignHealthy:
		return "D — foreign agent serving"
	case StateDisaster:
		return "E — disaster (multiple agents)"
	default:
		return "unknown"
	}
}

// answersWithNoProcessToShowForIt reports that the fixed endpoint answers and
// the process list holds nothing at all — a service serving a pipe, where there
// is no process to enumerate and reasoning from the list alone would call an
// answering agent no agent.
//
// It asks for an empty list rather than merely for no *reachable* agent,
// because a list that did name processes is information, and it has already
// been reasoned about: an agent belonging to another account is one such answer,
// and that an elevated caller can still reach its socket does not make it this
// account's agent.
func answersWithNoProcessToShowForIt(r Report) bool {
	return r.FixedReachable && len(r.Agents) == 0
}

// classifyState maps the gathered agents to a single lifecycle state, in the same
// precedence order the login path resolves: several live agents are a disaster, a
// lone healthy agent is ours or foreign, and with nothing live the question is
// only whether dead remnants of ours linger. An agent owned by a different real
// user never contributes: it cannot be ours, and — whether or not a privileged
// caller can technically still reach it — it is not serving this report's
// account, so it must not drive the state to foreign/disaster on that account's
// behalf.
func classifyState(r Report) State {
	var reachable, oursReach, otherReach, deadOurs int
	for _, a := range r.Agents {
		if differentUser(a, r.OurUID) {
			continue
		}
		if a.Reachable {
			reachable++
			if a.Kind == inspect.KindOurs {
				oursReach++
			} else {
				otherReach++
			}
			continue
		}
		if a.Socket != "" && (a.Kind == inspect.KindOurs || a.Kind == inspect.KindLegacy) {
			deadOurs++
		}
	}

	switch {
	case reachable > 1:
		return StateDisaster
	case oursReach == 1:
		return StateOursHealthy
	case answersWithNoProcessToShowForIt(r):
		return StateOursHealthy
	case otherReach == 1:
		return StateForeignHealthy
	case deadOurs > 0 || r.RecordedPID != 0:
		return StateOursZombie
	default:
		return StateClean
	}
}

// recommend returns the remediation for a state, phrased around what actually
// heals it today: opening a login shell, whose init reaps, starts, or adopts as
// the state requires.
//
// All of that rests on there being an agent to drive. Where this build has none
// on the system reported on, opening a shell changes nothing, and saying it
// would send somebody to do a thing that cannot work — so the answer there is
// about what is true rather than about what to do.
func recommend(r Report) string {
	if r.NoAgentMechanism {
		if r.State == StateOursHealthy || r.State == StateForeignHealthy {
			return "an agent is answering; SSHakku does not keep agents on this system yet, so nothing it does will change that"
		}
		return "no agent is answering, and SSHakku cannot start one on this system yet;" +
			" ssh will ask for each passphrase itself"
	}
	// A service nothing may start is not a state a login shell resolves. Every
	// answer below assumes a session opening can put things right, and here the
	// one thing that can is this command run with what it needs.
	if serviceIsDisabled(r) {
		return disabledServiceAdvice(r)
	}
	switch r.State {
	case StateClean:
		return "no agent is running; a new login shell starts one and loads your keys"
	case StateOursHealthy:
		return "the agent is healthy; no action needed"
	case StateOursZombie:
		return "a stale agent of ours is dead; a new login shell reaps it and restarts on the fixed socket"
	case StateForeignHealthy:
		return "a foreign agent is serving you; a new login shell adopts it and reports the anomaly — investigate who started it if this is unexpected"
	case StateDisaster:
		return "several agents are running at once; a new login shell settles on one healthy agent and reaps the dead"
	default:
		return ""
	}
}
