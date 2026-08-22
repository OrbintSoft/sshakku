//go:build windows

package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// F50, F51: what this build can do on the system it is running on is an answer,
// and it is asserted where the answer lives rather than assumed by whatever
// reads it. Here an agent is a service on an endpoint of the system's own, so
// there is something to keep healthy and something to start.
func TestThisSystemCanKeepAnAgentOnItsOwnEndpoint(t *testing.T) {
	assert.True(t, KeepsAgents(),
		"the lifecycle here has something to drive, and the callers act on this answer")
}

// F52: the other half of what this agent can do, and the one a configured key
// lifetime rests on. It cannot expire a key — asked to, it refuses the key
// altogether — which is why callers are told once rather than meeting it as a
// key that would not load.
func TestThisSystemsAgentHoldsNoKeyLifetimes(t *testing.T) {
	assert.False(t, KeepsLifetimes(),
		"a lifetime asked of this agent is refused, and what reads this answer acts on it")
}
