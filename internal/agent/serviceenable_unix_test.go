//go:build unix

package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/platform"
)

// F55: what the report says about the agent's service is what --fix acts on,
// and on a system whose agent is a process nobody starts a service. Enabling
// one has to answer that rather than report having done something, because a
// caller reaches it only by having been told a service was disabled — which
// nothing here ever says.
func TestEnablingAnAgentServiceHereSaysThereIsNoneToEnable(t *testing.T) {
	err := EnableAgentService(t.Context())

	require.Error(t, err, "a system with no service to enable must say so, not report success")
	assert.ErrorIs(t, err, platform.ErrUnimplemented,
		"the refusal must be the one a caller matches on, not a sentence it has to read")
	assert.ErrorContains(t, err, "enabling the agent's service",
		"and it must name what could not be done")
}
