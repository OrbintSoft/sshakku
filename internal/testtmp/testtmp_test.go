package testtmp

import (
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The directory handed out here is what every socket-binding test in the tree
// builds its address from. One that is merely "a temp dir" does not fail — it
// fails on macOS, in CI, in the tests above it, with an error about the socket
// rather than about the path it was given. That is what these tests are for.

func TestShortDirTakesASocketAddress(t *testing.T) {
	sock := filepath.Join(ShortDir(t), "agent.sock")

	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", sock)
	require.NoError(t, err, "the whole point of this directory is that a socket binds under it")
	require.NoError(t, ln.Close())
}

func TestShortDirIsFreshEachTime(t *testing.T) {
	first, second := ShortDir(t), ShortDir(t)

	assert.NotEqual(t, first, second,
		"two callers sharing a directory would see each other's sockets and files")
}

func TestShortDirIsRemovedWhenTheTestEnds(t *testing.T) {
	var dir string
	t.Run("inner", func(t *testing.T) {
		dir = ShortDir(t)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "left-behind"), []byte("x"), 0o600))
	})

	_, err := os.Stat(dir)
	assert.ErrorIs(t, err, os.ErrNotExist,
		"a directory left behind accumulates one per test run, under a path nothing cleans")
}
