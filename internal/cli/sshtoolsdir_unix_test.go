//go:build unix

package cli

import (
	"testing"

	"github.com/OrbintSoft/sshakku/internal/cli/shell"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNothingIsPutInFrontOfAnybodysOwnPathHere, whichever shell is asking. A
// directory put ahead of a user's PATH lasts their whole login and changes
// which ssh, scp and sftp they run, so it is worth doing only for a session
// that cannot otherwise reach its agent. There is one OpenSSH on such a system
// and the agent is a socket any build of it can open, so no session here is in
// that position and none is asked to change anything.
func TestNothingIsPutInFrontOfAnybodysOwnPathHere(t *testing.T) {
	for _, name := range []string{shell.Posix, shell.PowerShell} {
		t.Run(name, func(t *testing.T) {
			dialect, err := shell.Named(name)
			require.NoError(t, err, name)

			dir, err := sessionSSHToolsDir(t.Context(), dialect)

			require.NoError(t, err, "there is nothing on such a system that could fail to be decided")
			assert.Empty(t, dir, "a session here already runs tools that reach the agent it was pointed at")
		})
	}
}
