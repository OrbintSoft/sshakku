//go:build linux && backend_unresponsive

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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSecretServiceUnresponsiveDaemon covers a Secret Service daemon that is
// alive and still owns org.freedesktop.secrets but has stopped answering —
// distinct from the mid-session test's SIGKILL, which breaks the connection so
// godbus reports an error on its own. A frozen daemon keeps the connection
// open and never replies, the one case a deadline-less D-Bus call would wait
// on forever; the guarantee here is that a live round-trip, once the daemon is
// SIGSTOPped underneath it, surfaces a bounded error rather than hanging or
// returning a stale hit.
//
// SIGSTOP (not SIGKILL) is deliberate: a stopped process still owns the
// well-known name, so the bus cannot D-Bus-activate a replacement, and this
// test needs no activation-file removal the way the daemon-stopped mid-session
// scenario does. Each scenario is destructive to the shared session, so the
// build tag keeps this out of the ordinary `make test` and the desktop-stack
// job runs it in a throwaway container.
func TestSecretServiceUnresponsiveDaemon(t *testing.T) {
	if os.Getenv(allowRealSecretServiceEnv) == "" {
		t.Skipf("skipping: set %s=1 to run against a real Secret Service daemon (only safe in a disposable environment, e.g. a desktop-stack container)", allowRealSecretServiceEnv)
	}

	const (
		unresponsiveUser    = "sshakku-unresponsive-test-user"
		unresponsiveService = "sshakku-unresponsive-test-probe"
	)

	client, err := secretservice.NewClient(t.Context())
	if err != nil {
		t.Skipf("no real Secret Service daemon reachable on the ambient D-Bus session bus: %v", err)
	}
	t.Cleanup(func() { _ = client.Close(t.Context()) })

	backend := &SecretServiceBackend{Client: client, User: unresponsiveUser}
	require.NoError(t, backend.Unlock(t.Context()), "the wallet must open before anything can be frozen underneath it")
	t.Cleanup(func() { _ = backend.Delete(t.Context(), unresponsiveService) })

	// Prove the backend genuinely works before it is frozen, so the later
	// failure is a mid-session collapse rather than a backend that never
	// round-tripped.
	require.NoError(t, backend.Store(t.Context(), unresponsiveService, "sshakku unresponsive-daemon probe", "probe-passphrase"),
		"the wallet must genuinely work before it is frozen, or the later failure proves nothing")
	got, found, err := backend.Lookup(t.Context(), unresponsiveService)
	require.NoError(t, err, "and a passphrase just written must read back")
	require.True(t, found, "the entry is there, so it must be reported found")
	require.Equal(t, "probe-passphrase", got, "and it must be the one that was written")

	// Shorten the per-call deadline so the frozen-daemon Lookup fails in a few
	// seconds rather than the 30s default, still far above any healthy
	// round-trip. Set before the freeze; the value only bites on the next call.
	client.CallTimeout = 3 * time.Second

	require.NoError(t, signalProcessByComm("gnome-keyring-d", syscall.SIGSTOP),
		"freezing the Secret Service daemon is what this case is about")
	// Thaw in cleanup so the Delete/Close cleanups above can still reach the
	// bus, and nothing is left stopped even though the container is disposable.
	t.Cleanup(func() { _ = signalProcessByComm("gnome-keyring-d", syscall.SIGCONT) })

	err = lookupWithinBound(t, backend, unresponsiveService)
	assert.Error(t, err,
		"a wallet that stopped answering must surface an error the caller can fall back from, never a stale hit")
}

// lookupWithinBound runs one Lookup against the now-frozen backend and fails
// the test if it does not return within the outer bound — comfortably above the
// client's own CallTimeout, so a return here proves the deadline fired while a
// timeout here proves the call was not bounded at all and is hanging.
func lookupWithinBound(t *testing.T, backend *SecretServiceBackend, service string) error {
	t.Helper()
	type result struct {
		found bool
		err   error
	}
	done := make(chan result, 1)
	go func() {
		_, found, err := backend.Lookup(t.Context(), service)
		done <- result{found: found, err: err}
	}()
	select {
	case r := <-done:
		return r.err
	case <-time.After(15 * time.Second):
		require.FailNow(t, "a lookup against a frozen daemon is hanging",
			"it did not return within 15s, so the call is not bounded at all and the shell waiting on it never comes back")
		return nil
	}
}

// signalProcessByComm sends sig to the first process whose /proc comm matches
// name. The kernel truncates comm to 15 bytes, so callers pass the truncated
// form ("gnome-keyring-d" for gnome-keyring-daemon). It reads /proc directly
// rather than shelling out to pgrep, which the slim container images do not
// ship.
func signalProcessByComm(name string, sig syscall.Signal) error {
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
			return syscall.Kill(pid, sig)
		}
	}
	return os.ErrNotExist
}
