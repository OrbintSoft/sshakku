//go:build windows

package agent

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// F48: what this build has no implementation of here is said, not silently
// done badly.
//
// The refusal is the whole of the behaviour and it is the right one. Serialising
// the start path keeps two shells opening at the same moment from each starting
// an agent, and the alternative to refusing is handing back a release function
// that locks nothing — which every caller would then treat as a lock it holds.
// Proceeding unserialised is a decision with a cost, and it belongs to the
// caller, who can see what else is going on; it cannot be made here by handing
// back something that looks like success.
func TestALockThisSystemCannotTakeIsRefusedRatherThanFaked(t *testing.T) {
	release, err := FlockLocker{}.Lock(filepath.Join(t.TempDir(), "agent.lock"))

	require.Error(t, err, "there is no such lock here, and that is the answer")
	assert.Nil(t, release,
		"a release function would be a lock nobody holds, and every caller would hold it")
}
