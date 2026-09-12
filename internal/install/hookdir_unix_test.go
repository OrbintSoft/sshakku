//go:build unix

package install

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestADirectoryForTheRenderedHookIsMadeWhicheverScopeAsks. The hook is written
// into a directory that may not be there yet, and an install that wired a file
// to source a hook it had nowhere to put would fail at every login instead of
// at the moment somebody was watching. Both scopes are asked for here because
// on such a system they answer the same way — the machine's prefix belongs to
// the account a machine-wide install has to be made by, and no other account
// may write in it, so where the directory sits is already the answer that
// elsewhere has to be arranged for.
func TestADirectoryForTheRenderedHookIsMadeWhicheverScopeAsks(t *testing.T) {
	for _, scope := range []Scope{User, Machine} {
		t.Run(string(scope), func(t *testing.T) {
			// Two levels down, since the parent is as absent as the directory
			// itself when a scope's prefix has never been written in before.
			dir := filepath.Join(t.TempDir(), "share", "sshakku", "hooks")

			require.NoError(t, makeHookDirectory(scope, dir))

			info, err := os.Stat(dir)
			require.NoError(t, err, "the hook has somewhere to go once this has been asked")
			assert.True(t, info.IsDir(), "and what is there is a directory rather than something in its way")
		})
	}
}

// TestTheDirectoryTheMachineSharesMayBeReadByEveryAccount. A machine-wide hook
// is read at every login on the machine, by everyone, so a directory only its
// owner could enter would wire a file that every other account then fails to
// source. The mode is asserted on what is asked for rather than on what lands:
// the umask in force can only take bits away, and which bits it takes is the
// system's business rather than this call's.
func TestTheDirectoryTheMachineSharesMayBeReadByEveryAccount(t *testing.T) {
	// The bits that let every account read the directory and enter it, and the
	// ones that would let somebody other than the installer write in it.
	const (
		readAndEnterByAnybody = 0o055
		writtenBySomebodyElse = 0o022
	)

	assert.Zero(t, readAndEnterByAnybody&^hookDirMode,
		"every account on the machine has to be able to read this directory and enter it")
	assert.Zero(t, hookDirMode&writtenBySomebodyElse,
		"and none but the account that installed may write in it")

	dir := filepath.Join(t.TempDir(), "hooks")
	require.NoError(t, makeDirectoryTheMachineShares(dir))
	assert.DirExists(t, dir, "which is a directory this system makes the ordinary way")
}
