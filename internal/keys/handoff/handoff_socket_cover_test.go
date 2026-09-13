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

// The socket rendezvous is not one system's (handoff_socket.go), so neither is
// what proves it refuses. Every failure below is one of the shared steps
// declining, and each system that reaches for this rendezvous has to decline the
// same way: what is left of a stash that could not be finished is a passphrase
// somewhere it was not meant to be, and that is the same mistake everywhere.
//
// What differs between the systems is only how a thing is made private, and the
// tests for that live with the system that decides it.

// addrLimit is the address length these tests hold themselves to: the strictest
// of the systems this runs on, so a socket path that fits here fits everywhere.
// The production values live with the platform that imposes them.
const addrLimit = 103

// fixedBase answers with one directory, for the tests whose subject is what
// happens once the base is known rather than how one is chosen.
func fixedBase(dir string) func() (string, error) {
	return func() (string, error) { return dir, nil }
}

// The failures these tests hand their seams. Each stands for a real one the
// code under test cannot be made to produce on demand.
var (
	errChmodBoom  = errors.New("chmod boom")
	errListenBoom = errors.New("listen boom")
	errNoBase     = errors.New("no base")
	errReadBoom   = errors.New("read boom")
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
	readAll = func(io.Reader) ([]byte, error) { return nil, errReadBoom }
	_, err = socketHandoffFetch(t.Context(), token)
	assert.Error(t, err, "a passphrase that could not be read must be reported, not handed on as an empty one")
}

// TestSocketHandoffServedNothingIsNotAPassphrase covers what a collector is
// told when the rendezvous is still there to dial but hands nothing over — the
// state a stash is in between serving its one connection and taking itself
// away, which is what a second collector finds if it arrives inside that gap.
// Nothing handed over and a passphrase the user left empty are the same string
// and different events, and only the second one is an answer.
func TestSocketHandoffServedNothingIsNotAPassphrase(t *testing.T) {
	sock := filepath.Join(testtmp.ShortDir(t), "collected.sock")
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", sock)
	require.NoError(t, err, "a rendezvous to collect from must be there")
	t.Cleanup(func() { _ = ln.Close() })

	// Serves what a stash whose passphrase has already been taken serves: the
	// connection is accepted and closed with no byte across it.
	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr != nil {
			return
		}
		_ = conn.Close()
	}()

	_, err = socketHandoffFetch(t.Context(), sock)
	assert.Error(t, err,
		"a handoff that handed nothing over must be reported as that, not passed on as a passphrase")
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
		chmodDir = func(string, os.FileMode) error { return errChmodBoom }
		_, err := socketHandoffDir(testtmp.ShortDir(t))
		assert.Error(t, err,
			"a directory that could not be made private must not be used: a passphrase would rendezvous in the open")
	})
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
			return "", errNoBase
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
		randRead = func([]byte) (int, error) { return 0, errRngBoom }
		_, err := socketHandoffStash("s", time.Second, fixedBase(testtmp.ShortDir(t)), addrLimit)
		assert.Error(t, err, "a rendezvous another process could guess the name of must not be opened")
	})

	t.Run("listen fails", func(t *testing.T) {
		saveHandoffSocketSeams(t)
		netListen = func(string, string) (net.Listener, error) { return nil, errListenBoom }
		_, err := socketHandoffStash("s", time.Second, fixedBase(testtmp.ShortDir(t)), addrLimit)
		assert.Error(t, err, "a rendezvous nothing is listening at cannot hand anything over")
	})

	t.Run("chmod fails and the socket is cleaned up", func(t *testing.T) {
		saveHandoffSocketSeams(t)
		chmodSock = func(string, os.FileMode) error { return errChmodBoom }
		_, err := socketHandoffStash("s", time.Second, fixedBase(testtmp.ShortDir(t)), addrLimit)
		assert.Error(t, err,
			"a socket that could not be made private must not be left serving: anything that connects gets the passphrase")
	})
}
