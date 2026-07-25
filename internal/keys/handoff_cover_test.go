package keys

import (
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// saveHandoffSocketSeams snapshots the RNG, listen, and chmod seams shared by
// the token and socket-handoff code, restoring them when the (sub)test ends.
func saveHandoffSocketSeams(t *testing.T) {
	t.Helper()
	oRand, oListen, oChmod, oRead := randRead, netListen, chmodSock, readAll
	t.Cleanup(func() { randRead, netListen, chmodSock, readAll = oRand, oListen, oChmod, oRead })
}

func TestRandomHandoffTokenReadError(t *testing.T) {
	saveHandoffSocketSeams(t)
	randRead = func([]byte) (int, error) { return 0, errors.New("rng boom") }
	if _, err := randomHandoffToken(); err == nil {
		t.Fatal("randomHandoffToken returned nil error, want the RNG failure")
	}
}

// TestFetchHandoffMalformedToken covers FetchHandoff (and its platform
// fetchPassphrase) rejecting a token it cannot redeem: a non-numeric keyring
// serial on Linux, an undialable socket path on Darwin.
func TestFetchHandoffMalformedToken(t *testing.T) {
	if _, err := FetchHandoff("definitely-not-a-valid-handoff-token"); err == nil {
		t.Fatal("FetchHandoff returned nil error, want a redemption failure")
	}
}

func TestSocketHandoffFetchDialError(t *testing.T) {
	if _, err := socketHandoffFetch(filepath.Join(t.TempDir(), "nope.sock")); err == nil {
		t.Fatal("socketHandoffFetch returned nil error, want a dial failure")
	}
}

// TestSocketHandoffFetchReadError covers the branch where the socket dials
// successfully but reading the served passphrase fails.
func TestSocketHandoffFetchReadError(t *testing.T) {
	t.Setenv("HOME", shortDir(t))
	t.Setenv("XDG_CACHE_HOME", "")
	saveHandoffSocketSeams(t)

	token, err := socketHandoffStash("s3cr3t", 5*time.Second)
	if err != nil {
		t.Fatalf("socketHandoffStash: %v", err)
	}
	readAll = func(io.Reader) ([]byte, error) { return nil, errors.New("read boom") }
	if _, err := socketHandoffFetch(token); err == nil {
		t.Fatal("socketHandoffFetch returned nil error, want the read failure")
	}
}

func TestSocketHandoffDirErrors(t *testing.T) {
	t.Run("no cache directory resolvable", func(t *testing.T) {
		t.Setenv("HOME", "")
		t.Setenv("XDG_CACHE_HOME", "")
		if _, err := socketHandoffDir(); err == nil {
			t.Fatal("socketHandoffDir returned nil error, want the no-cache-dir failure")
		}
	})

	t.Run("cache path is a file, not a directory", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "cache-file")
		if err := os.WriteFile(file, nil, 0o600); err != nil {
			t.Fatalf("write file: %v", err)
		}
		t.Setenv("XDG_CACHE_HOME", file)
		if _, err := socketHandoffDir(); err == nil {
			t.Fatal("socketHandoffDir returned nil error, want the mkdir failure under a file")
		}
	})
}

func TestSocketHandoffStashErrors(t *testing.T) {
	t.Run("cache directory unresolvable", func(t *testing.T) {
		t.Setenv("HOME", "")
		t.Setenv("XDG_CACHE_HOME", "")
		if _, err := socketHandoffStash("s", time.Second); err == nil {
			t.Fatal("socketHandoffStash returned nil error, want the dir failure")
		}
	})

	t.Run("token RNG fails", func(t *testing.T) {
		t.Setenv("HOME", shortDir(t))
		t.Setenv("XDG_CACHE_HOME", "")
		saveHandoffSocketSeams(t)
		randRead = func([]byte) (int, error) { return 0, errors.New("rng boom") }
		if _, err := socketHandoffStash("s", time.Second); err == nil {
			t.Fatal("socketHandoffStash returned nil error, want the RNG failure")
		}
	})

	t.Run("listen fails", func(t *testing.T) {
		t.Setenv("HOME", shortDir(t))
		t.Setenv("XDG_CACHE_HOME", "")
		saveHandoffSocketSeams(t)
		netListen = func(string, string) (net.Listener, error) { return nil, errors.New("listen boom") }
		if _, err := socketHandoffStash("s", time.Second); err == nil {
			t.Fatal("socketHandoffStash returned nil error, want the listen failure")
		}
	})

	t.Run("chmod fails and the socket is cleaned up", func(t *testing.T) {
		t.Setenv("HOME", shortDir(t))
		t.Setenv("XDG_CACHE_HOME", "")
		saveHandoffSocketSeams(t)
		chmodSock = func(string, os.FileMode) error { return errors.New("chmod boom") }
		if _, err := socketHandoffStash("s", time.Second); err == nil {
			t.Fatal("socketHandoffStash returned nil error, want the chmod failure")
		}
	})
}
