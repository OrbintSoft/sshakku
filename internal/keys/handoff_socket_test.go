package keys

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// addrLimit is the address length these tests hold themselves to: the stricter
// of the two systems this runs on, so a socket path that fits here fits
// everywhere. The production values live with the platform that imposes them.
const addrLimit = 103

// shortDir returns a fresh temp dir under /tmp — not t.TempDir()'s nested,
// test-name-derived path, and not os.MkdirTemp("", ...)'s default either: on
// Darwin that resolves under $TMPDIR, itself a long per-boot randomized path
// (/var/folders/.../T/), and a test that spends its budget on the directory
// leading up to the socket has nothing left to say about the socket. /tmp is
// short on both.
func shortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "sshakku")
	require.NoError(t, err, "a short directory to put the socket in")
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// fixedBase answers with one directory, for the tests whose subject is what
// happens once the base is known rather than how one is chosen.
func fixedBase(dir string) func() (string, error) {
	return func() (string, error) { return dir, nil }
}

func TestSocketHandoffRoundTrip(t *testing.T) {
	token, err := socketHandoffStash("s3cr3t", 5*time.Second, fixedBase(shortDir(t)), addrLimit)
	require.NoError(t, err, "putting a passphrase aside for the helper to collect must succeed")

	info, err := os.Stat(token)
	require.NoError(t, err, "and the socket it will be collected from must be there")
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(),
		"reachable by this user alone: anyone who can connect to it gets the passphrase")

	got, err := socketHandoffFetch(token)
	require.NoError(t, err, "collecting it must succeed")
	assert.Equal(t, "s3cr3t", got, "and hand back exactly what was put aside")
}

func TestSocketHandoffOneShot(t *testing.T) {
	token, err := socketHandoffStash("s3cr3t", 5*time.Second, fixedBase(shortDir(t)), addrLimit)
	require.NoError(t, err, "putting a passphrase aside must succeed")
	_, err = socketHandoffFetch(token)
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

	_, err = socketHandoffFetch(token)
	assert.Error(t, err, "and a second attempt must get nothing: one stash is one handoff")
}

func TestSocketHandoffExpiresUnclaimed(t *testing.T) {
	token, err := socketHandoffStash("s3cr3t", 100*time.Millisecond, fixedBase(shortDir(t)), addrLimit)
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
