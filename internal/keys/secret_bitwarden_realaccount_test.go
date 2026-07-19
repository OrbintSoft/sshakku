//go:build unix

package keys

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

// allowRealBitwardenEnv opts this test into driving a real bw CLI through
// BitwardenBackend's own Unlock/Lock — including the master-password prompt
// — against a real Bitwarden (or self-hosted Vaultwarden) account. Unlike
// the 1Password real-account test, there is no OS-integrated unlock to rely
// on (see the design note on BitwardenBackend), so this test needs the
// account's identity up front: SSHAKKU_TEST_BW_EMAIL and
// SSHAKKU_TEST_BW_PASSWORD (a fixed, disposable-fixture password — never a
// real secret; see vaultwarden-session.sh) and, for a self-hosted
// server, SSHAKKU_TEST_BW_SERVER. It never runs `op signin`-equivalent setup
// itself beyond that: Unlock is the exact production code path, exercised
// here for real rather than assumed correct because it round-trips through
// a fake Runner in secret_bitwarden_test.go. It only runs locally or in the
// tier-2 container for now (see PLAN.md 4.2) — a Bitwarden/Vaultwarden
// account needs a server to talk to, unlike the local `op` app-integration
// case.
const allowRealBitwardenEnv = "SSHAKKU_TEST_ALLOW_REAL_BITWARDEN"

// TestBitwardenBackendRealAccount exercises BitwardenBackend end to end
// against a real bw CLI, driving Unlock/Lock itself (via a fixed-answer
// Prompter, never a real interactive one) rather than receiving an
// already-unlocked session — unlike secret_bitwarden_test.go, which only
// ever talks to a fake Runner. It creates its own throwaway item, named
// with a timestamp so it can never collide with or touch an existing one,
// and deletes it in t.Cleanup regardless of outcome.
func TestBitwardenBackendRealAccount(t *testing.T) {
	if os.Getenv(allowRealBitwardenEnv) == "" {
		t.Skipf("skipping: set %s=1 plus SSHAKKU_TEST_BW_EMAIL/SSHAKKU_TEST_BW_PASSWORD (and optionally SSHAKKU_TEST_BW_SERVER) to run against a real bw account (see PLAN.md 4.2)", allowRealBitwardenEnv)
	}
	if _, err := exec.LookPath(bitwardenBin); err != nil {
		t.Skipf("bw CLI not found: %v", err)
	}
	email := os.Getenv("SSHAKKU_TEST_BW_EMAIL")
	password := os.Getenv("SSHAKKU_TEST_BW_PASSWORD")
	if email == "" || password == "" {
		t.Skip("SSHAKKU_TEST_BW_EMAIL and SSHAKKU_TEST_BW_PASSWORD must both be set")
	}

	backend := &BitwardenBackend{
		Runner:   ExecRunner{},
		Prompter: &fakePrompter{pass: password},
		Email:    email,
		Server:   os.Getenv("SSHAKKU_TEST_BW_SERVER"),
	}

	testService := "sshakku-integration-test-probe-" + time.Now().UTC().Format("20060102T150405.000000000")
	const (
		testLabel = "sshakku integration test probe"
		testPass  = "probe-passphrase-not-a-real-secret"
	)
	t.Cleanup(func() { _ = backend.Delete(testService) })

	// Each call below unlocks and locks for itself (held stays false), the
	// same standalone bracket the reactive askpass-broker path uses — so
	// this also proves a *repeated* fresh master-password prompt/unlock
	// works against a real daemon, not just once.
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
