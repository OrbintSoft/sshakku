//go:build unix

package sshtools

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNothingHereEmulatesAnotherSystem. There is one OpenSSH on such a machine
// and the agent is a socket any build of it can open, so there is nothing to
// choose between and no reason to go looking: the program the session finds is
// the program that runs.
func TestNothingHereEmulatesAnotherSystem(t *testing.T) {
	assert.Empty(t, ThisSystem().EmulationRuntimes)
	assert.Empty(t, ThisSystem().NativeDirs)
}

// TestTheNameIsHandedBackUntouchedHere, so a user's own build of OpenSSH — one
// earlier on their PATH than the packaged one — goes on being the one SSHakku
// drives, exactly as it is the one their shell drives.
func TestTheNameIsHandedBackUntouchedHere(t *testing.T) {
	tool, err := ThisSystem().Tool(SSHAddName)

	require.NoError(t, err)
	assert.Equal(t, SSHAddName, tool)
}
