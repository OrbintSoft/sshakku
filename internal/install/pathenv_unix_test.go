//go:build unix

package install

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This system keeps no stored environment for an install to change, and saying
// so is an answer rather than a step that was skipped. It is asserted because
// the install reports what this returns: a true here would have it claim to
// have changed a search list that nothing on this system reads.
func TestNothingIsRecordedInASearchListOnThisSystem(t *testing.T) {
	for _, scope := range []Scope{User, Machine} {
		added, err := AddToPath(scope, "/home/example/.local/bin")
		require.NoError(t, err)
		assert.False(t, added, "%s: there is nowhere to record one, so nothing changed", scope)

		removed, err := RemoveFromPath(scope, "/home/example/.local/bin")
		require.NoError(t, err)
		assert.False(t, removed, "%s: nothing put one there, so there is nothing to take out", scope)
	}

	assert.NotEmpty(t, PathStepNothingToDo, "the install has to have something true to report")
}
