//go:build unix

package handoff

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/testtmp"
)

// TestSocketHandoffDirIsPrivate covers what the directory is for: a rendezvous
// for a passphrase, which nobody but its owner may enter — whatever the umask
// of the process that created it happened to be.
//
// This is this family's answer and not the other's: the bits are what say who
// may enter here, and forcing them is what makes a directory that was already
// there, with looser permissions, private before it is used. A system that
// synthesises these bits from a read-only attribute says nothing about who may
// enter in them, and is asked the same question its own way
// (handoff_privacy_windows.go).
func TestSocketHandoffDirIsPrivate(t *testing.T) {
	dir, err := socketHandoffDir(testtmp.ShortDir(t))
	require.NoError(t, err, "making the rendezvous directory must succeed")
	info, err := os.Stat(dir)
	require.NoError(t, err, "and it must be there")
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm(),
		"enterable by its owner alone, whatever umask the process that made it happened to have")
}
