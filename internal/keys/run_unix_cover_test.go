//go:build unix

package keys

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBoundToProcessGroupCancelBeforeStart drives the deadline against a command
// that was never started, the race the nil check guards: the context can expire
// between building the Cmd and exec forking it, and Cancel then runs with no
// process behind it.
//
// Without the guard, reading the pid off the nil Process panics, and it panics
// inside the goroutine os/exec runs Cancel on — nothing there recovers, so it
// takes the whole program down rather than failing one command. That is the
// difference the guard makes: a command SSHakku could not start becomes a
// command that simply did not run.
func TestBoundToProcessGroupCancelBeforeStart(t *testing.T) {
	cmd := exec.Command("true")
	boundToProcessGroup(cmd)

	require.NotNil(t, cmd.Cancel, "the deadline has to have something to call when it fires")
	assert.NoError(t, cmd.Cancel(),
		"and cancelling a command that never started must simply do nothing: os/exec runs this on a goroutine "+
			"that recovers from nothing, so a panic here takes the whole program down")
}
