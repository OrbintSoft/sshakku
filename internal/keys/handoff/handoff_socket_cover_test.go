package handoff

import (
	"context"
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

	errDeadlineBoom = errors.New("deadline boom")
)

// saveHandoffSocketSeams snapshots the RNG, listen, chmod, read and deadline
// seams shared by the token and socket-handoff code — and the collecting budget
// beside them, which a test shortens for the same reason it swaps a seam —
// restoring them when the (sub)test ends.
func saveHandoffSocketSeams(t *testing.T) {
	t.Helper()
	oRand, oListen, oChmodDir, oChmod, oRead := randRead, netListen, chmodDir, chmodSock, readAll
	oDeadline, oBudget := setDeadline, fetchBudget
	t.Cleanup(func() {
		randRead, netListen, chmodDir, chmodSock, readAll = oRand, oListen, oChmodDir, oChmod, oRead
		setDeadline, fetchBudget = oDeadline, oBudget
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
	// The budget is shortened rather than left at its own value: where closing
	// a socket does not reach the other end as an end of file, what ends this
	// read is the budget, and a test must not spend the real one to find out.
	saveHandoffSocketSeams(t)
	fetchBudget = 100 * time.Millisecond

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

// TestSocketHandoffFetchGivesUpOnARendezvousThatSaysNothing verifies F21 on the
// handoff: nothing SSHakku waits on may hold a shell up with no end, and a
// rendezvous that neither answers nor fails is exactly the shape that promise
// is about. Behind this read are sshakku-askpass, the ssh-add waiting on it,
// and the login shell waiting on that.
//
// The peer accepts and then says nothing at all, which hangs on every platform
// rather than only where closing an AF_UNIX socket fails to deliver an end of
// file — the state this was first seen in is one system's way of reaching it,
// not the fault itself.
func TestSocketHandoffFetchGivesUpOnARendezvousThatSaysNothing(t *testing.T) {
	saveHandoffSocketSeams(t)
	fetchBudget = 100 * time.Millisecond

	sock := filepath.Join(testtmp.ShortDir(t), "silent.sock")
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", sock)
	require.NoError(t, err, "a rendezvous to collect from must be there")
	t.Cleanup(func() { _ = ln.Close() })

	// Accepted and then held open, with no byte written and no close: a server
	// that has stopped answering without having failed.
	held := make(chan net.Conn, 1)
	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr != nil {
			return
		}
		held <- conn
	}()
	t.Cleanup(func() {
		select {
		case conn := <-held:
			_ = conn.Close()
		default:
		}
	})

	start := time.Now()
	_, err = socketHandoffFetch(t.Context(), sock)
	assert.Error(t, err, "a rendezvous that says nothing must be given up on, not waited on")
	assert.Less(t, time.Since(start), time.Minute,
		"the waiting must end on its own, and not because something else came along and ended it")
}

// TestSocketHandoffFetchHonoursTheCallersOwnDeadline verifies that a caller who
// has already stopped waiting is not left holding a read that has not: the
// sooner of the two moments is the one collecting gives up at.
func TestSocketHandoffFetchHonoursTheCallersOwnDeadline(t *testing.T) {
	saveHandoffSocketSeams(t)
	fetchBudget = time.Hour

	var asked time.Time
	setDeadline = func(_ net.Conn, at time.Time) error {
		asked = at
		return nil
	}

	token, err := socketHandoffStash("s3cr3t", 5*time.Second, fixedBase(testtmp.ShortDir(t)), addrLimit)
	require.NoError(t, err, "putting a passphrase aside must succeed")

	theirs := time.Now().Add(time.Second)
	ctx, cancel := context.WithDeadline(t.Context(), theirs)
	defer cancel()
	_, err = socketHandoffFetch(ctx, token)
	require.NoError(t, err, "the passphrase is there to collect")
	assert.WithinDuration(t, theirs, asked, time.Millisecond,
		"the caller's own deadline falls first, so it is the one collecting stops at")
}

// TestSocketHandoffFetchDeadlineError covers the branch where the collecting
// budget cannot be put on the connection at all. Reading on without it would be
// the unbounded wait this exists to prevent, so it is refused instead.
func TestSocketHandoffFetchDeadlineError(t *testing.T) {
	saveHandoffSocketSeams(t)
	setDeadline = func(net.Conn, time.Time) error { return errDeadlineBoom }

	token, err := socketHandoffStash("s3cr3t", 5*time.Second, fixedBase(testtmp.ShortDir(t)), addrLimit)
	require.NoError(t, err, "putting a passphrase aside must succeed")
	_, err = socketHandoffFetch(t.Context(), token)
	assert.ErrorIs(t, err, errDeadlineBoom, "a read that cannot be bounded must not be made anyway")
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
