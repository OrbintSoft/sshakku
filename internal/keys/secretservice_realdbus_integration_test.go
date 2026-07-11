//go:build unix

package keys

import (
	"os"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/secretservice"
)

// allowRealSecretServiceEnv opts this test into touching whatever Secret
// Service daemon owns org.freedesktop.secrets on the ambient D-Bus session
// bus. On a developer's machine that ambient bus is their real, live desktop
// session — there is no way to tell a disposable one from a real one from
// inside the test — so this must default to skipped and only ever be set by
// an environment built to be disposable, e.g. the tier-2 container's
// entrypoint, never by a developer running `go test` locally.
const allowRealSecretServiceEnv = "SSHAKKU_TEST_ALLOW_REAL_SECRETSERVICE"

// TestSecretServiceBackendRealDaemon exercises SecretServiceBackend end to
// end against a real Secret Service daemon (ksecretd, GNOME Keyring, ...) —
// unlike internal/secretservice's own tests, which only ever talk to an
// in-process fake peer. It proves the exact wiring cmd/sshakku's
// newSecretBackend uses in production round-trips through a real backend,
// not just the fake one.
//
// A throwaway username/service pair keeps it from ever touching real
// sshakku-managed items even when it does run, and it deletes what it
// created regardless of outcome — but see allowRealSecretServiceEnv for why
// it still must not run outside a disposable environment: creating the
// "sshakku" collection the first time relies on that environment's ksecretd
// having "sshakku" pre-configured as its PAM-opened default wallet (see
// phase4-tier2-kde-steps.md), which a real desktop session never has.
//
// It drives the whole test through one SecretSession Unlock/Lock, exactly
// as Loader does for a batch of keys, rather than each call's own implicit
// unlock/lock: ksecretd genuinely re-locks (closes) the wallet on every
// explicit Lock, and re-unlocking a already-created, non-default-wallet
// collection needs a real interactive dialog every time — something only a
// batched single unlock avoids.
func TestSecretServiceBackendRealDaemon(t *testing.T) {
	if os.Getenv(allowRealSecretServiceEnv) == "" {
		t.Skipf("skipping: set %s=1 to run against a real Secret Service daemon (only safe in a disposable environment, e.g. the tier-2 container)", allowRealSecretServiceEnv)
	}

	client, err := secretservice.NewClient()
	if err != nil {
		t.Skipf("no real Secret Service daemon reachable on the ambient D-Bus session bus: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	const (
		testUser    = "sshakku-integration-test-user"
		testService = "sshakku-integration-test-probe"
	)
	backend := &SecretServiceBackend{Client: client, User: testUser}
	if err := backend.Unlock(); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	// Registered before the Delete cleanup below so it runs last (t.Cleanup
	// is LIFO): Delete must still run while held unlocked.
	t.Cleanup(func() { _ = backend.Lock() })
	t.Cleanup(func() { _ = backend.Delete(testService) })

	if err := backend.Store(testService, "sshakku integration test probe", "probe-passphrase"); err != nil {
		t.Fatalf("Store: %v", err)
	}

	got, found, err := backend.Lookup(testService)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if !found {
		t.Fatal("Lookup: not found immediately after Store")
	}
	if got != "probe-passphrase" {
		t.Fatalf("Lookup passphrase = %q, want %q", got, "probe-passphrase")
	}

	if err := backend.Delete(testService); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, found, err := backend.Lookup(testService); err != nil {
		t.Fatalf("Lookup after Delete: %v", err)
	} else if found {
		t.Fatal("Lookup after Delete: still found")
	}
}
