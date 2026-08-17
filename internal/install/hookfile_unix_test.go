//go:build unix

package install

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bashCandidates says where this system keeps a bash that can run the shell
// library. Here it is whichever one the PATH names, which is the shell a user
// of this system would get.
func bashCandidates(t *testing.T) []string {
	t.Helper()
	return []string{"bash"}
}

// Who may read a startup file is a question only a system with real permission
// bits can be asked, which is why these two live here. On Windows the bits are
// synthesised and a test of them would be reading its own reflection.

// The file a hook is wired into belongs to somebody else, and a machine-wide
// one has to stay readable by every account that logs in — it is read at every
// login, by everyone. Writing through a temporary file is what makes the
// replacement atomic; it must not also be what decides who may read the
// result, and a temporary file is private to its owner by default.
func TestUpsertBlockFileKeepsThePermissionsItFound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bash.bashrc")
	require.NoError(t, os.WriteFile(path, []byte("umask 022\n"), 0o644))
	require.NoError(t, os.Chmod(path, 0o644))

	require.NoError(t, UpsertBlockFile(path, ". \"/hook.sh\""))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o644), info.Mode().Perm(),
		"a startup file nobody but root can read is one that silently stops working at login")
}

// A file this program had to create is one nothing else has an opinion about
// yet, so it is given what a startup file normally has rather than what a
// temporary file happens to be created with.
func TestUpsertBlockFileCreatesAFileEveryoneCanRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile")

	require.NoError(t, UpsertBlockFile(path, ". \"/hook.sh\""))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o644), info.Mode().Perm())
}

func TestDropInIsExecutable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "001-sshakku-init.sh")

	require.NoError(t, WriteDropIn(path, BourneDropIn(". \"/hook.sh\"")))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o755), info.Mode().Perm(),
		"a file in a drop-in directory is run, not sourced, by some of the shells that read one")
}

// The realistic way an unwiring fails: a machine-wide startup file, and an
// account without the privilege to change what is in that directory. Both shapes
// of wiring are refused by name rather than reported as removed — somebody who is
// told the hook is gone and finds it still running at the next login has been
// told the one thing that must be true.
func TestAWiringInADirectoryThisAccountMayNotWriteIsReported(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("this account may write anywhere, so there is no refusal to be had")
	}

	dir := t.TempDir()
	block := filepath.Join(dir, "zprofile")
	require.NoError(t, os.WriteFile(block, UpsertBlock(nil, ". \"/hook.sh\""), 0o644))
	dropIn := filepath.Join(dir, "50-sshakku-init.sh")
	require.NoError(t, os.WriteFile(dropIn, BourneDropIn(". \"/hook.sh\""), 0o755))

	require.NoError(t, os.Chmod(dir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	// The file holds nothing but the wiring, so unwiring it means removing it —
	// which this account may not do.
	err := StripBlockFile(block)
	require.Error(t, err)
	assert.Contains(t, err.Error(), block)

	err = RemoveDropIn(dropIn)
	require.Error(t, err)
	assert.Contains(t, err.Error(), dropIn)
}
