//go:build windows

package inspect

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAgentsRefusesRatherThanAnswerEmpty pins the shape of the refusal, not
// just its presence. A caller asks for the process list to decide whether to
// start an agent, so an empty list and no error would be acted on as "none is
// running" — the one answer this platform must never give while it cannot
// read a process's argv.
func TestAgentsRefusesRatherThanAnswerEmpty(t *testing.T) {
	procs, err := Inspector{}.Agents()
	require.Error(t, err, "a platform that cannot enumerate agents must say so")
	assert.Nil(t, procs, "no process list may be returned alongside the refusal")
}
