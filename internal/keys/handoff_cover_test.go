package keys

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

func TestRandomHandoffTokenReadError(t *testing.T) {
	saveHandoffSocketSeams(t)
	randRead = func([]byte) (int, error) { return 0, errors.New("rng boom") }
	_, err := randomHandoffToken()
	assert.Error(t, err,
		"a token that is not random is one another process can guess, so an RNG that failed must stop the handoff")
}

// TestFetchHandoffMalformedToken covers FetchHandoff (and its platform
// fetchPassphrase) rejecting a token it cannot redeem: a non-numeric keyring
// serial on Linux, an undialable socket path on Darwin.
func TestFetchHandoffMalformedToken(t *testing.T) {
	_, err := FetchHandoff("definitely-not-a-valid-handoff-token")
	assert.Error(t, err, "a handle no stash was made under can redeem nothing")
}

func TestSocketHandoffFetchDialError(t *testing.T) {
	_, err := socketHandoffFetch(filepath.Join(t.TempDir(), "nope.sock"))
	assert.Error(t, err, "a rendezvous that is not there hands back nothing")
}

// TestSocketHandoffFetchReadError covers the branch where the socket dials
// successfully but reading the served passphrase fails.
func TestSocketHandoffFetchReadError(t *testing.T) {
	saveHandoffSocketSeams(t)

	token, err := socketHandoffStash("s3cr3t", 5*time.Second, fixedBase(shortDir(t)), addrLimit)
	require.NoError(t, err, "putting a passphrase aside must succeed")
	readAll = func(io.Reader) ([]byte, error) { return nil, errors.New("read boom") }
	_, err = socketHandoffFetch(token)
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
		_, err := socketHandoffDir(shortDir(t))
		assert.Error(t, err,
			"a directory that could not be made private must not be used: a passphrase would rendezvous in the open")
	})
}

// TestSocketHandoffDirIsPrivate covers what the directory is for: a rendezvous
// for a passphrase, which nobody but its owner may enter — whatever the umask
// of the process that created it happened to be.
func TestSocketHandoffDirIsPrivate(t *testing.T) {
	dir, err := socketHandoffDir(shortDir(t))
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
	_, err := socketHandoffStash("s", time.Second, fixedBase(shortDir(t)), 20)
	require.Error(t, err, "an address the kernel would refuse must be caught before it is offered")
	assert.Contains(t, err.Error(), "socket address",
		"and say what was too long: the kernel's own answer is \"invalid argument\", which names nothing")
	assert.Contains(t, err.Error(), "20", "and what the limit was")
}

// TestChooseSocketBase covers which directory a passphrase rendezvous is
// allowed to be made in: the short per-user one when it really is private,
// and the fallback whenever anything about that is not so.
func TestChooseSocketBase(t *testing.T) {
	cache := func() (string, error) { return "/the/cache", nil }

	t.Run("a private temporary directory is preferred", func(t *testing.T) {
		got, err := chooseSocketBase("/the/tmp", func(string) bool { return true }, cache)
		require.NoError(t, err, "choosing where the rendezvous goes must succeed")
		assert.Equal(t, "/the/tmp", got, "a short private directory keeps the socket address inside the kernel's limit")
	})

	t.Run("a shared temporary directory is refused", func(t *testing.T) {
		got, err := chooseSocketBase("/the/tmp", func(string) bool { return false }, cache)
		require.NoError(t, err, "choosing where the rendezvous goes must succeed")
		assert.Equal(t, "/the/cache", got,
			"a directory anyone else can reach is no place for a passphrase, however short its name")
	})

	t.Run("no temporary directory named at all", func(t *testing.T) {
		got, err := chooseSocketBase("", func(string) bool {
			assert.Fail(t, "a directory nobody named was inspected", "there is nothing there to ask about")
			return true
		}, cache)
		require.NoError(t, err, "choosing where the rendezvous goes must succeed")
		assert.Equal(t, "/the/cache", got, "with no temporary directory named, the user's own cache is where it goes")
	})

	t.Run("and no cache directory either", func(t *testing.T) {
		_, err := chooseSocketBase("", func(string) bool { return true }, func() (string, error) {
			return "", errors.New("no home")
		})
		assert.Error(t, err, "with nowhere private to put it, the handoff must not happen at all")
	})
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
		_, err := socketHandoffStash("s", time.Second, fixedBase(shortDir(t)), addrLimit)
		assert.Error(t, err, "a rendezvous another process could guess the name of must not be opened")
	})

	t.Run("listen fails", func(t *testing.T) {
		saveHandoffSocketSeams(t)
		netListen = func(string, string) (net.Listener, error) { return nil, errors.New("listen boom") }
		_, err := socketHandoffStash("s", time.Second, fixedBase(shortDir(t)), addrLimit)
		assert.Error(t, err, "a rendezvous nothing is listening at cannot hand anything over")
	})

	t.Run("chmod fails and the socket is cleaned up", func(t *testing.T) {
		saveHandoffSocketSeams(t)
		chmodSock = func(string, os.FileMode) error { return errors.New("chmod boom") }
		_, err := socketHandoffStash("s", time.Second, fixedBase(shortDir(t)), addrLimit)
		assert.Error(t, err,
			"a socket that could not be made private must not be left serving: anything that connects gets the passphrase")
	})
}
