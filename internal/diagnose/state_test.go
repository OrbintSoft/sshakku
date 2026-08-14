package diagnose

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/OrbintSoft/sshakku/internal/agent/inspect"
)

func TestClassifyState(t *testing.T) {
	ours := func(reachable bool) AgentView {
		return AgentView{Kind: inspect.KindOurs, Socket: fixed, Reachable: reachable}
	}
	foreign := func(reachable bool) AgentView {
		return AgentView{Kind: inspect.KindForeign, Socket: "/tmp/f.sock", Reachable: reachable}
	}
	legacyAgent := AgentView{Kind: inspect.KindLegacy, Socket: legacy + "/ssh-agent.sock", Reachable: true}

	cases := []struct {
		name string
		r    Report
		want State
	}{
		{"clean, nothing", Report{}, StateClean},
		{"clean, only a dead foreign", Report{Agents: []AgentView{foreign(false)}}, StateClean},
		{"ours healthy", Report{Agents: []AgentView{ours(true)}}, StateOursHealthy},
		{"foreign healthy", Report{Agents: []AgentView{foreign(true)}}, StateForeignHealthy},
		{"legacy healthy counts as foreign", Report{Agents: []AgentView{legacyAgent}}, StateForeignHealthy},
		{"ours zombie, dead socket", Report{Agents: []AgentView{ours(false)}}, StateOursZombie},
		{
			// What makes a dead agent worth reporting is the stale socket it
			// left behind. A process whose socket could not be determined has
			// left nothing to clear up, and calling it a remnant would send the
			// user to open a login shell to reap something that is not there.
			"a dead agent that left no socket is not a remnant to reap",
			Report{Agents: []AgentView{{Kind: inspect.KindOurs, Socket: "", Reachable: false}}},
			StateClean,
		},
		{"ours zombie, recorded pid only", Report{RecordedPID: 123}, StateOursZombie},
		{"disaster, two live", Report{Agents: []AgentView{ours(true), foreign(true)}}, StateDisaster},
		{
			// A process whose owner could not be read is not thereby somebody
			// else's: excluded on that basis it would vanish from the report,
			// and a report that quietly leaves agents out is worse than one
			// that names an agent it is unsure about.
			"an agent whose owner could not be determined still counts as this account's",
			Report{OurUID: 1000, Agents: []AgentView{{Kind: inspect.KindOurs, UID: -1, Socket: fixed, Reachable: true}}},
			StateOursHealthy,
		},
		{
			"a different user's healthy agent doesn't make it foreign-serving",
			Report{OurUID: 0, Agents: []AgentView{{Kind: inspect.KindForeign, UID: 1000, Socket: "/tmp/f.sock", Reachable: true}}},
			StateClean,
		},
		{
			"a different user's agent doesn't mask our own healthy one either",
			Report{OurUID: 0, Agents: []AgentView{ours(true), {Kind: inspect.KindForeign, UID: 1000, Socket: "/tmp/f.sock", Reachable: true}}},
			StateOursHealthy,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, classifyState(c.r), "the state this report describes")
		})
	}
}

func TestRecommend(t *testing.T) {
	// Every state a report can reach is one the user is told what to do about;
	// assert, not require, so a run names all the silent ones rather than the
	// first.
	for _, s := range []State{StateClean, StateOursHealthy, StateOursZombie, StateForeignHealthy, StateDisaster} {
		assert.NotEmptyf(t, recommend(s), "%v must come with something to do about it", s)
	}
	assert.Empty(t, recommend(StateUnknown), "a state that was never worked out has nothing to advise")
}

func TestStateString(t *testing.T) {
	cases := map[State]string{
		StateClean:          "A —",
		StateOursHealthy:    "B —",
		StateOursZombie:     "C —",
		StateForeignHealthy: "D —",
		StateDisaster:       "E —",
		StateUnknown:        "unknown",
	}
	for s, prefix := range cases {
		got := s.String()
		assert.Truef(t, strings.HasPrefix(got, prefix), "State(%d).String() = %q, want it to start with %q", s, got, prefix)
	}
}
