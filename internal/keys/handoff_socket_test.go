package keys

import (
	"os"
	"testing"
	"time"
)

// shortDir returns a fresh temp dir under /tmp — not t.TempDir()'s nested,
// test-name-derived path, and not os.MkdirTemp("", ...)'s default either: on
// Darwin that resolves under $TMPDIR, itself a long per-boot randomized path
// (/var/folders/.../T/), which combined with this package's own
// cache-dir/socket-name suffix still overflows AF_UNIX's sun_path limit (108
// bytes on Linux, 104 on Darwin). /tmp is short on both.
func shortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "sshakku")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func TestSocketHandoffRoundTrip(t *testing.T) {
	t.Setenv("HOME", shortDir(t))
	t.Setenv("XDG_CACHE_HOME", "")

	token, err := socketHandoffStash("s3cr3t", 5*time.Second)
	if err != nil {
		t.Fatalf("socketHandoffStash: %v", err)
	}

	info, err := os.Stat(token)
	if err != nil {
		t.Fatalf("stat socket before fetch: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("socket permissions = %o, want 0600", perm)
	}

	got, err := socketHandoffFetch(token)
	if err != nil {
		t.Fatalf("socketHandoffFetch: %v", err)
	}
	if got != "s3cr3t" {
		t.Fatalf("socketHandoffFetch = %q, want %q", got, "s3cr3t")
	}
}

func TestSocketHandoffOneShot(t *testing.T) {
	t.Setenv("HOME", shortDir(t))
	t.Setenv("XDG_CACHE_HOME", "")

	token, err := socketHandoffStash("s3cr3t", 5*time.Second)
	if err != nil {
		t.Fatalf("socketHandoffStash: %v", err)
	}
	if _, err := socketHandoffFetch(token); err != nil {
		t.Fatalf("first socketHandoffFetch: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(token); os.IsNotExist(err) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := os.Stat(token); !os.IsNotExist(err) {
		t.Fatalf("socket %s still exists after the one-shot fetch", token)
	}

	if _, err := socketHandoffFetch(token); err == nil {
		t.Fatal("second socketHandoffFetch succeeded, want an error (one-shot stash already served)")
	}
}

func TestSocketHandoffExpiresUnclaimed(t *testing.T) {
	t.Setenv("HOME", shortDir(t))
	t.Setenv("XDG_CACHE_HOME", "")

	token, err := socketHandoffStash("s3cr3t", 100*time.Millisecond)
	if err != nil {
		t.Fatalf("socketHandoffStash: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(token); os.IsNotExist(err) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("socket %s was never cleaned up after its ttl elapsed unclaimed", token)
}
