//go:build unix

package install

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This system is the thing the other one emulates, so a shell and this program
// spell a path the same way. It is asserted rather than left to a test that
// quietly declines to run: a translator turning up here would mean paths were
// about to be rewritten that must be passed through whole.
func TestPathsAreWrittenAsTheyAreOnThisSystem(t *testing.T) {
	s := spellingFor(filepath.Join("/usr", "bin", "bash"))

	forShell, err := s.forShell(t.Context(), "/home/o'brien/.local/share/sshakku/shell-hook.sh")
	require.NoError(t, err)
	assert.Equal(t, "/home/o'brien/.local/share/sshakku/shell-hook.sh", forShell)

	forUs, err := s.forUs(t.Context(), "/home/o'brien/.profile")
	require.NoError(t, err)
	assert.Equal(t, "/home/o'brien/.profile", forUs)
}
