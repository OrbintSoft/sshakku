//go:build linux && midsession_failure

package keys

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/OrbintSoft/sshakku/internal/secretservice"
)

// TestSecretServiceMidSessionFailure exercises what happens to a live
// SecretServiceBackend when the machinery underneath it disappears partway
// through a session — the daemon crashing, or the whole D-Bus session bus
// going away — rather than the tidy connect/use/disconnect the other
// real-daemon test covers. Each scenario is destructive to the shared
// session, so this must run one scenario per throwaway container (the
// desktop-stack job selects a single subtest with -run); the build tag keeps
// it out of the ordinary `make test`, where killing the daemon would break
// every sibling test in the same package run.
//
// The guarantee under test is that a mid-session collapse surfaces as a
// prompt error, never a hang and never a silently-wrong success: a bounded
// wait around each post-collapse call catches a regression that would block
// forever, and the error itself proves the backend did not carry on as if the
// store were still there.
func TestSecretServiceMidSessionFailure(t *testing.T) {
	if os.Getenv(allowRealSecretServiceEnv) == "" {
		t.Skipf("skipping: set %s=1 to run against a real Secret Service daemon (only safe in a disposable environment, e.g. a desktop-stack container)", allowRealSecretServiceEnv)
	}

	// The daemon-stopped scenario relies on the bus NOT respawning the daemon
	// it just watched die; the desktop-stack job removes the activation .service
	// file to arrange that. Refuse to run it otherwise rather than pass on a
	// respawned daemon that never actually went away.
	t.Run("daemon stopped mid-session", func(t *testing.T) {
		if os.Getenv("SSHAKKU_DISABLE_SECRETS_ACTIVATION") == "" {
			t.Skip("skipping: needs SSHAKKU_DISABLE_SECRETS_ACTIVATION=1 so the bus does not respawn the killed daemon")
		}
		backend := establishLiveSession(t)

		if err := killProcessByComm("gnome-keyring-d"); err != nil {
			t.Fatalf("killing the Secret Service daemon: %v", err)
		}

		err := lookupWithinTimeout(t, backend)
		if err == nil {
			t.Fatal("Lookup succeeded after the daemon was killed; a dead backend must surface an error, not a stale hit")
		}
		t.Logf("post-crash Lookup returned the expected error: %v", err)
	})

	// Killing the bus itself pulls the transport out from under the client's
	// existing connection — a strictly lower-level failure than losing the
	// daemon that was speaking over it.
	t.Run("session bus unreachable mid-session", func(t *testing.T) {
		backend := establishLiveSession(t)

		if err := killProcessByComm("dbus-daemon"); err != nil {
			t.Fatalf("killing the D-Bus session bus: %v", err)
		}

		err := lookupWithinTimeout(t, backend)
		if err == nil {
			t.Fatal("Lookup succeeded after the session bus was killed; a severed connection must surface an error")
		}
		t.Logf("post-bus-loss Lookup returned the expected error: %v", err)
	})
}

const (
	midSessionUser    = "sshakku-midsession-test-user"
	midSessionService = "sshakku-midsession-test-probe"
)

// establishLiveSession builds the exact backend production wires up, proves it
// round-trips against the real daemon (so the later failure is genuinely
// mid-session rather than a backend that never worked), and returns it held
// unlocked for the caller to break underneath.
func establishLiveSession(t *testing.T) *SecretServiceBackend {
	t.Helper()

	client, err := secretservice.NewClient()
	if err != nil {
		t.Skipf("no real Secret Service daemon reachable on the ambient D-Bus session bus: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	backend := &SecretServiceBackend{Client: client, User: midSessionUser}
	if err := backend.Unlock(); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	t.Cleanup(func() { _ = backend.Delete(midSessionService) })

	if err := backend.Store(midSessionService, "sshakku mid-session failure probe", "probe-passphrase"); err != nil {
		t.Fatalf("Store: %v", err)
	}
	got, found, err := backend.Lookup(midSessionService)
	if err != nil || !found || got != "probe-passphrase" {
		t.Fatalf("pre-failure Lookup = (%q, %v, %v), want (%q, true, nil)", got, found, err, "probe-passphrase")
	}
	return backend
}

// lookupWithinTimeout runs one Lookup against the now-broken backend and fails
// the test if it does not return within the bound — the point of the whole
// exercise is that a collapsed backend errors promptly instead of blocking the
// caller forever.
func lookupWithinTimeout(t *testing.T, backend *SecretServiceBackend) error {
	t.Helper()
	type result struct {
		found bool
		err   error
	}
	done := make(chan result, 1)
	go func() {
		_, found, err := backend.Lookup(midSessionService)
		done <- result{found: found, err: err}
	}()
	select {
	case r := <-done:
		return r.err
	case <-time.After(15 * time.Second):
		t.Fatal("Lookup did not return within 15s after the backend was killed; it is hanging")
		return nil
	}
}

// killProcessByComm SIGKILLs the first process whose /proc comm matches name.
// The kernel truncates comm to 15 bytes, so callers pass the truncated form
// ("gnome-keyring-d" for gnome-keyring-daemon). It reads /proc directly rather
// than shelling out to pgrep, which the slim container images do not ship.
func killProcessByComm(name string) error {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return err
	}
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue // not a pid directory
		}
		comm, err := os.ReadFile(filepath.Join("/proc", e.Name(), "comm"))
		if err != nil {
			continue // process gone or unreadable; keep scanning
		}
		if strings.TrimSpace(string(comm)) == name {
			return syscall.Kill(pid, syscall.SIGKILL)
		}
	}
	return os.ErrNotExist
}
