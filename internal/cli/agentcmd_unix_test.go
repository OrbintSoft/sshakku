//go:build unix

package cli

import (
	"testing"

	"github.com/OrbintSoft/sshakku/internal/agent"
	"github.com/stretchr/testify/assert"
)

// F1, F2: what drives the agent on this system is the lifecycle that starts one
// on a socket of its own choosing, reaps what has died and adopts what it did
// not start. The counterpart on the other platform names its own, so neither
// answer is left to whichever machine happens to run the suite.
func TestThisSystemsLifecycleIsTheSocketOne(t *testing.T) {
	assert.IsType(t, agent.Manager{}, platformEnsurer(),
		"an agent here is a process on a socket, and there is a lifecycle to keep it")
	assert.IsType(t, agent.Manager{}, realEnsurer(),
		"and that is what the product composes here")
}
