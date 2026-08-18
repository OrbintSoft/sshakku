//go:build unix

package handoff

import (
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/testtmp"
)

// saveHandoffSocketSeams snapshots the RNG, listen, and chmod seams shared by
// the token and socket-handoff code, restoring them when the (sub)test ends.
func saveHandoffSocketSeams(t *testing.T) {
	t.Helper()
	oRand, oListen, oChmodDir, oChmod, oRead := randRead, netListen, chmodDir, chmodSock, readAll
	t.Cleanup(func() {
		randRead, netListen, chmodDir, chmodSock, readAll = oRand, oListen, oChmodDir, oChmod, oRead
	})
}

func TestSocketHandoffFetchDialError(t *testing.T) {
	_, err := socketHandoffFetch(t.Context(), filepath.Join(t.TempDir(), "nope.sock"))
	assert.Error(t, err, "a rendezvous that is not there hands back nothing")
}

// TestSocketHandoffFetchReadError covers the branch where the socket dials
// successfully but reading the served passphrase fails.
func TestSocketHandoffFetchReadError(t *testing.T) {
	saveHandoffSocketSeams(t)

	token, err := socketHandoffStash("s3cr3t", 5*time.Second, fixedBase(testtmp.ShortDir(t)), addrLimit)
	require.NoError(t, err, "putting a passphrase aside must succeed")
	readAll = func(io.Reader) ([]byte, error) { return nil, errors.New("read boom") }
	_, err = socketHandoffFetch(t.Context(), token)
	assert.Error(t, err, "a passphrase that could not be read must be reported, not handed on as an empty one")
}

func TestSocketHandoffDirErrors(t *testing.T) {
	t.Run("the base is a file, not a directory", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "not-a-dir")
		require.NoError(t, os.WriteFile(file, nil, 0o600), "seed a file where a directory should be")
		_, err := socketHandoffDir(file)
		assert.Error(t, err, "there is nowhere to put the rendezvous, and that must be said")
	})

	t.Run("the directory cannot be made private", func(t *testing.T) {
		saveHandoffSocketSeams(t)
		chmodDir = func(string, os.FileMode) error { return errors.New("chmod boom") }
		_, err := socketHandoffDir(testtmp.ShortDir(t))
		assert.Error(t, err,
			"a directory that could not be made private must not be used: a passphrase would rendezvous in the open")
	})
}

// TestSocketHandoffDirIsPrivate covers what the directory is for: a rendezvous
// for a passphrase, which nobody but its owner may enter — whatever the umask
// of the process that created it happened to be.
func TestSocketHandoffDirIsPrivate(t *testing.T) {
	dir, err := socketHandoffDir(testtmp.ShortDir(t))
	require.NoError(t, err, "making the rendezvous directory must succeed")
	info, err := os.Stat(dir)
	require.NoError(t, err, "and it must be there")
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm(),
		"enterable by its owner alone, whatever umask the process that made it happened to have")
}

// TestSocketHandoffAddressTooLong covers the guard on an address the kernel
// would refuse: what comes back has to name the length and the limit, since
// the kernel's own answer ("invalid argument") names neither.
func TestSocketHandoffAddressTooLong(t *testing.T) {
	_, err := socketHandoffStash("s", time.Second, fixedBase(testtmp.ShortDir(t)), 20)
	require.Error(t, err, "an address the kernel would refuse must be caught before it is offered")
	assert.Contains(t, err.Error(), "socket address",
		"and say what was too long: the kernel's own answer is \"invalid argument\", which names nothing")
	assert.Contains(t, err.Error(), "20", "and what the limit was")
}

func TestSocketHandoffStashErrors(t *testing.T) {
	t.Run("the base cannot be resolved at all", func(t *testing.T) {
		_, err := socketHandoffStash("s", time.Second, func() (string, error) {
			return "", errors.New("no base")
		}, addrLimit)
		assert.Error(t, err, "with nowhere to put the passphrase, it must not be put anywhere")
	})

	t.Run("the directory cannot be made", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "not-a-dir")
		require.NoError(t, os.WriteFile(file, nil, 0o600), "seed a file where a directory should be")
		_, err := socketHandoffStash("s", time.Second, fixedBase(file), addrLimit)
		assert.Error(t, err, "with nowhere to put the passphrase, it must not be put anywhere")
	})

	t.Run("token RNG fails", func(t *testing.T) {
		saveHandoffSocketSeams(t)
		randRead = func([]byte) (int, error) { return 0, errors.New("rng boom") }
		_, err := socketHandoffStash("s", time.Second, fixedBase(testtmp.ShortDir(t)), addrLimit)
		assert.Error(t, err, "a rendezvous another process could guess the name of must not be opened")
	})

	t.Run("listen fails", func(t *testing.T) {
		saveHandoffSocketSeams(t)
		netListen = func(string, string) (net.Listener, error) { return nil, errors.New("listen boom") }
		_, err := socketHandoffStash("s", time.Second, fixedBase(testtmp.ShortDir(t)), addrLimit)
		assert.Error(t, err, "a rendezvous nothing is listening at cannot hand anything over")
	})

	t.Run("chmod fails and the socket is cleaned up", func(t *testing.T) {
		saveHandoffSocketSeams(t)
		chmodSock = func(string, os.FileMode) error { return errors.New("chmod boom") }
		_, err := socketHandoffStash("s", time.Second, fixedBase(testtmp.ShortDir(t)), addrLimit)
		assert.Error(t, err,
			"a socket that could not be made private must not be left serving: anything that connects gets the passphrase")
	})
}
