package agent

import (
	"testing"

	"github.com/OrbintSoft/sshakku/internal/platform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// F48: on a system with no way to keep an agent, the answer is given once and
// up front rather than discovered by walking into whichever piece happens to be
// missing first. What the caller acts on is the sentinel; the words are for
// whoever reads the log afterwards.
func TestASystemWithNoAgentMechanismSaysSoInsteadOfDrivingOne(t *testing.T) {
	result, err := NoMechanism{}.EnsureAgent(t.Context(), EnsureConfig{FixedSock: "/nowhere/agent.sock"}, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, platform.ErrUnimplemented)
	assert.Contains(t, err.Error(), "ssh-agent", "the words name the piece that is missing")
	assert.Empty(t, result.LiveSock, "and no socket is offered for an agent that was never started")
}
