//go:build unix

package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// F48: what this build can do on the system it is running on is an answer, and
// it is asserted where the answer lives rather than assumed by whatever reads
// it. Here an agent is a process listening on a socket whose path is handed to
// it, so an endpoint can be fixed and kept — which is what the whole of this
// package is for, and a build that said otherwise would switch all of it off.
func TestThisSystemCanKeepAnAgentOnAFixedEndpoint(t *testing.T) {
	assert.True(t, KeepsAgents(),
		"the lifecycle in this package has something to drive here, and the callers act on this answer")
}

// F52: and what a configured key lifetime rests on here — the agent holds a key
// for the time it was given and then drops it, which is what `ssh-add -t` asks
// for. The other platform's answer is asserted in its own file.
func TestThisSystemsAgentHoldsKeyLifetimes(t *testing.T) {
	assert.True(t, KeepsLifetimes(),
		"a lifetime asked of this agent is honoured, and what reads this answer acts on it")
}
