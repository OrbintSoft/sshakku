//go:build unix

package keys

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// allowRealBitwardenEnv opts this test into exercising a real Bitwarden (or
// self-hosted Vaultwarden) account through a BW_SESSION that is already
// unlocked. Unlike the 1Password real-account test, this one never obtains
// its own session — bw login and bw unlock need the account's master
// password, which this test must never accept, log, or print — so that
// step is entirely the caller's responsibility: the tier-2 container's
// vaultwarden-tier2-session.sh does it against a disposable, pre-registered
// fixture account before running the test suite (see PLAN.md 4.2 for why
// account registration itself can't be automated here). A developer could
// run it the same way locally against their own bw login/unlock, but there
// is no tier-1-style bare-account path — a Bitwarden/Vaultwarden account
// needs a server to talk to, unlike the local `op` app-integration case.
const allowRealBitwardenEnv = "SSHAKKU_TEST_ALLOW_REAL_BITWARDEN"

// TestBitwardenBackendRealAccount exercises BitwardenBackend end to end
// against a real bw CLI and BW_SESSION — unlike secret_bitwarden_test.go,
// which only ever talks to a fake Runner. It creates its own throwaway item,
// named with a timestamp so it can never collide with or touch an existing
// one, and deletes it in t.Cleanup regardless of outcome.
func TestBitwardenBackendRealAccount(t *testing.T) {
	if os.Getenv(allowRealBitwardenEnv) == "" {
		t.Skipf("skipping: set %s=1 and BW_SESSION to an already-unlocked session to run against a real bw account (see PLAN.md 4.2)", allowRealBitwardenEnv)
	}
	if _, err := exec.LookPath(bitwardenBin); err != nil {
		t.Skipf("bw CLI not found: %v", err)
	}
	session := os.Getenv("BW_SESSION")
	if session == "" {
		t.Skip("BW_SESSION is not set — bw login/unlock must already have run (see vaultwarden-tier2-session.sh)")
	}
	if out, err := exec.Command(bitwardenBin, "status", "--session", session).CombinedOutput(); err != nil || !strings.Contains(string(out), `"status":"unlocked"`) {
		t.Skipf("bw is not unlocked: %v: %s", err, strings.TrimSpace(string(out)))
	}

	backend := &BitwardenBackend{Runner: ExecRunner{}, Session: session}

	testService := "sshakku-integration-test-probe-" + time.Now().UTC().Format("20060102T150405.000000000")
	const (
		testLabel = "sshakku integration test probe"
		testPass  = "probe-passphrase-not-a-real-secret"
	)
	t.Cleanup(func() { _ = backend.Delete(testService) })

	if err := backend.Store(testService, testLabel, testPass); err != nil {
		t.Fatalf("Store: %v", err)
	}

	got, found, err := backend.Lookup(testService)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if !found {
		t.Fatal("Lookup: not found immediately after Store")
	}
	if got != testPass {
		t.Fatalf("Lookup passphrase = %q, want %q", got, testPass)
	}

	services, err := backend.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !containsString(services, testService) {
		t.Fatalf("List = %v, want it to contain %q", services, testService)
	}

	const updatedPass = "probe-passphrase-updated-not-a-real-secret"
	if err := backend.Store(testService, testLabel, updatedPass); err != nil {
		t.Fatalf("Store (update): %v", err)
	}
	if got, _, err := backend.Lookup(testService); err != nil {
		t.Fatalf("Lookup after update: %v", err)
	} else if got != updatedPass {
		t.Fatalf("Lookup after update = %q, want %q", got, updatedPass)
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
