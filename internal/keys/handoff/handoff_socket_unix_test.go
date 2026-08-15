//go:build unix

package handoff

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/testtmp"
)

// addrLimit is the address length these tests hold themselves to: the stricter
// of the two systems this runs on, so a socket path that fits here fits
// everywhere. The production values live with the platform that imposes them.
const addrLimit = 103

// fixedBase answers with one directory, for the tests whose subject is what
// happens once the base is known rather than how one is chosen.
func fixedBase(dir string) func() (string, error) {
	return func() (string, error) { return dir, nil }
}

func TestSocketHandoffRoundTrip(t *testing.T) {
	token, err := socketHandoffStash("s3cr3t", 5*time.Second, fixedBase(testtmp.ShortDir(t)), addrLimit)
	require.NoError(t, err, "putting a passphrase aside for the helper to collect must succeed")

	info, err := os.Stat(token)
	require.NoError(t, err, "and the socket it will be collected from must be there")
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(),
		"reachable by this user alone: anyone who can connect to it gets the passphrase")

	got, err := socketHandoffFetch(t.Context(), token)
	require.NoError(t, err, "collecting it must succeed")
	assert.Equal(t, "s3cr3t", got, "and hand back exactly what was put aside")
}

func TestSocketHandoffOneShot(t *testing.T) {
	token, err := socketHandoffStash("s3cr3t", 5*time.Second, fixedBase(testtmp.ShortDir(t)), addrLimit)
	require.NoError(t, err, "putting a passphrase aside must succeed")
	_, err = socketHandoffFetch(t.Context(), token)
	require.NoError(t, err, "and the helper that was meant to have it must get it")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(token); os.IsNotExist(err) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	_, err = os.Stat(token)
	assert.Truef(t, os.IsNotExist(err),
		"a passphrase that has been collected must leave nothing behind to collect again: %s", token)

	_, err = socketHandoffFetch(t.Context(), token)
	assert.Error(t, err, "and a second attempt must get nothing: one stash is one handoff")
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
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		_ = conn.Close()
	}()

	_, err = socketHandoffFetch(t.Context(), sock)
	assert.Error(t, err,
		"a handoff that handed nothing over must be reported as that, not passed on as a passphrase")
}

func TestSocketHandoffExpiresUnclaimed(t *testing.T) {
	token, err := socketHandoffStash("s3cr3t", 100*time.Millisecond, fixedBase(testtmp.ShortDir(t)), addrLimit)
	require.NoError(t, err, "putting a passphrase aside must succeed")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(token); os.IsNotExist(err) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	require.FailNowf(t, "a passphrase nobody collected was left where it was put",
		"the socket %s outlived its own deadline, so the secret sits there for anything that can reach it", token)
}
