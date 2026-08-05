package keys

import (
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	saveHandoffSocketSeams(t)

	token, err := socketHandoffStash("s3cr3t", 5*time.Second, fixedBase(shortDir(t)), addrLimit)
	if err != nil {
		t.Fatalf("socketHandoffStash: %v", err)
	}
	readAll = func(io.Reader) ([]byte, error) { return nil, errors.New("read boom") }
	if _, err := socketHandoffFetch(token); err == nil {
		t.Fatal("socketHandoffFetch returned nil error, want the read failure")
	}
}

func TestSocketHandoffDirErrors(t *testing.T) {
	t.Run("the base is a file, not a directory", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "not-a-dir")
		if err := os.WriteFile(file, nil, 0o600); err != nil {
			t.Fatalf("write file: %v", err)
		}
		if _, err := socketHandoffDir(file); err == nil {
			t.Fatal("socketHandoffDir returned nil error, want the mkdir failure under a file")
		}
	})

	t.Run("the directory cannot be made private", func(t *testing.T) {
		saveHandoffSocketSeams(t)
		chmodDir = func(string, os.FileMode) error { return errors.New("chmod boom") }
		if _, err := socketHandoffDir(shortDir(t)); err == nil {
			t.Fatal("socketHandoffDir returned nil error, want the failure to force 0700")
		}
	})
}

// TestSocketHandoffDirIsPrivate covers what the directory is for: a rendezvous
// for a passphrase, which nobody but its owner may enter — whatever the umask
// of the process that created it happened to be.
func TestSocketHandoffDirIsPrivate(t *testing.T) {
	dir, err := socketHandoffDir(shortDir(t))
	if err != nil {
		t.Fatalf("socketHandoffDir: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat handoff dir: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Fatalf("handoff directory permissions = %o, want 0700", perm)
	}
}

// TestSocketHandoffAddressTooLong covers the guard on an address the kernel
// would refuse: what comes back has to name the length and the limit, since
// the kernel's own answer ("invalid argument") names neither.
func TestSocketHandoffAddressTooLong(t *testing.T) {
	_, err := socketHandoffStash("s", time.Second, fixedBase(shortDir(t)), 20)
	if err == nil {
		t.Fatal("socketHandoffStash returned nil error, want the address-too-long failure")
	}
	for _, want := range []string{"socket address", "20"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not mention %q", err, want)
		}
	}
}

// TestChooseSocketBase covers which directory a passphrase rendezvous is
// allowed to be made in: the short per-user one when it really is private,
// and the fallback whenever anything about that is not so.
func TestChooseSocketBase(t *testing.T) {
	cache := func() (string, error) { return "/the/cache", nil }

	t.Run("a private temporary directory is preferred", func(t *testing.T) {
		got, err := chooseSocketBase("/the/tmp", func(string) bool { return true }, cache)
		if err != nil || got != "/the/tmp" {
			t.Fatalf("chooseSocketBase = %q, %v; want the temporary directory", got, err)
		}
	})

	t.Run("a shared temporary directory is refused", func(t *testing.T) {
		got, err := chooseSocketBase("/the/tmp", func(string) bool { return false }, cache)
		if err != nil || got != "/the/cache" {
			t.Fatalf("chooseSocketBase = %q, %v; want the cache directory", got, err)
		}
	})

	t.Run("no temporary directory named at all", func(t *testing.T) {
		got, err := chooseSocketBase("", func(string) bool {
			t.Fatal("an unnamed directory was inspected")
			return true
		}, cache)
		if err != nil || got != "/the/cache" {
			t.Fatalf("chooseSocketBase = %q, %v; want the cache directory", got, err)
		}
	})

	t.Run("and no cache directory either", func(t *testing.T) {
		if _, err := chooseSocketBase("", func(string) bool { return true }, func() (string, error) {
			return "", errors.New("no home")
		}); err == nil {
			t.Fatal("chooseSocketBase returned nil error, want the failure from the cache directory")
		}
	})
}

func TestSocketHandoffStashErrors(t *testing.T) {
	t.Run("the base cannot be resolved at all", func(t *testing.T) {
		if _, err := socketHandoffStash("s", time.Second, func() (string, error) {
			return "", errors.New("no base")
		}, addrLimit); err == nil {
			t.Fatal("socketHandoffStash returned nil error, want the failure from resolving a base")
		}
	})

	t.Run("the directory cannot be made", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "not-a-dir")
		if err := os.WriteFile(file, nil, 0o600); err != nil {
			t.Fatalf("write file: %v", err)
		}
		if _, err := socketHandoffStash("s", time.Second, fixedBase(file), addrLimit); err == nil {
			t.Fatal("socketHandoffStash returned nil error, want the dir failure")
		}
	})

	t.Run("token RNG fails", func(t *testing.T) {
		saveHandoffSocketSeams(t)
		randRead = func([]byte) (int, error) { return 0, errors.New("rng boom") }
		if _, err := socketHandoffStash("s", time.Second, fixedBase(shortDir(t)), addrLimit); err == nil {
			t.Fatal("socketHandoffStash returned nil error, want the RNG failure")
		}
	})

	t.Run("listen fails", func(t *testing.T) {
		saveHandoffSocketSeams(t)
		netListen = func(string, string) (net.Listener, error) { return nil, errors.New("listen boom") }
		if _, err := socketHandoffStash("s", time.Second, fixedBase(shortDir(t)), addrLimit); err == nil {
			t.Fatal("socketHandoffStash returned nil error, want the listen failure")
		}
	})

	t.Run("chmod fails and the socket is cleaned up", func(t *testing.T) {
		saveHandoffSocketSeams(t)
		chmodSock = func(string, os.FileMode) error { return errors.New("chmod boom") }
		if _, err := socketHandoffStash("s", time.Second, fixedBase(shortDir(t)), addrLimit); err == nil {
			t.Fatal("socketHandoffStash returned nil error, want the chmod failure")
		}
	})
}
