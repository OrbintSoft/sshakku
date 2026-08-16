package diagnose

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// F48: where this build has no way to keep an ssh-agent, the report says so
// and stops sending somebody to do the thing that would have worked elsewhere.
//
// Both answers are checked from either platform on purpose: which one a machine
// gets is stated by the caller that knows, and nothing here reads the system it
// is running on. A report that could only be checked from the machine it
// describes would be one half of this permanently untested.

func TestASystemThatCannotKeepAnAgentIsNotToldToOpenALoginShell(t *testing.T) {
	advice := recommend(Report{State: StateClean, NoAgentMechanism: true})

	assert.NotContains(t, advice, "login shell",
		"no shell opening here starts an agent, so saying so sends somebody to do nothing")
	assert.NotEmpty(t, advice, "but there is still something true to say")
}

func TestASystemThatCanKeepAnAgentIsStillToldToOpenOne(t *testing.T) {
	assert.Contains(t, recommend(Report{State: StateClean}), "login shell",
		"which is what actually heals this state where the mechanism exists")
}

func TestAnAgentThatIsAnsweringIsLeftAloneWhereNothingManagesAgents(t *testing.T) {
	advice := recommend(Report{State: StateForeignHealthy, NoAgentMechanism: true})

	assert.NotContains(t, advice, "login shell", "nothing here adopts an agent")
	assert.NotEmpty(t, advice)
}

func TestTheFindingForNoAgentSaysWhetherOneCanBeStartedHere(t *testing.T) {
	here := strings.Join(findings(Inputs{}, Report{NoAgentMechanism: true}), "\n")
	elsewhere := strings.Join(findings(Inputs{}, Report{}), "\n")

	assert.Contains(t, elsewhere, "a new login shell will start one",
		"where one can be started, that is what to do about it")
	assert.NotContains(t, here, "will start one",
		"and where none can, promising it is the report telling you something untrue")
	assert.Contains(t, here, "no ssh-agent is answering",
		"what is observed is the same either way")
}
