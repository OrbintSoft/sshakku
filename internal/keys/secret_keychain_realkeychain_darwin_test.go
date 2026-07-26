//go:build darwin

package keys

import (
	"os"
	"testing"
	"time"
)

// allowRealKeychainEnv opts this test into reading and writing the real macOS
// keychain through DarwinKeychainClient's live Security.framework calls —
// unlike secret_keychain_test.go, which only ever talks to a fake
// KeychainClient. It must default to skipped: the calls hit whatever keychain
// is current default for the process, an OS-level side effect a unit test
// can't stand in for. It writes only items under a unique, timestamped account
// and deletes every one of them when the test ends, so it leaves no trace, but
// the target keychain is still real. CI points the default keychain at a
// throwaway one first (test/macos-keychain-setup.sh) so the runner's login
// keychain is never touched; a developer running this locally opts into their
// own default keychain knowingly.
const allowRealKeychainEnv = "SSHAKKU_TEST_ALLOW_REAL_KEYCHAIN"

// TestDarwinKeychainClientRealRoundTrip exercises DarwinKeychainClient end to
// end against a live keychain: the full Add / Find / Update / Delete / List
// happy path plus the two error paths the fake can't reproduce faithfully —
// adding a duplicate item and updating one that doesn't exist. It asserts only
// on observable Go behaviour (returned values, the found bool, whether err is
// nil), never on specific OSStatus numbers, so it stays a black-box check of
// the client's contract.
//
// The keychain is live external state go test's cache can't see, so a repeat
// run with allowRealKeychainEnv unchanged can replay a cached pass. Pass
// -count=1 to force a real run.
func TestDarwinKeychainClientRealRoundTrip(t *testing.T) {
	if os.Getenv(allowRealKeychainEnv) == "" {
		t.Skipf("skipping: set %s=1 to run against the real macOS keychain (writes only timestamped throwaway items, deletes them after)", allowRealKeychainEnv)
	}

	c := DarwinKeychainClient{}
	stamp := time.Now().UTC().Format("20060102T150405.000000000")
	account := "sshakku-integration-test-" + stamp
	service := "svc-" + stamp
	missing := "svc-missing-" + stamp

	// Delete both names up front and again on exit, no matter how the test
	// leaves them: Delete is idempotent, so this both guarantees a clean start
	// and guarantees nothing is left behind.
	clean := func() {
		if err := c.Delete(account, service); err != nil {
			t.Errorf("cleanup Delete(%q): %v", service, err)
		}
		if err := c.Delete(account, missing); err != nil {
			t.Errorf("cleanup Delete(%q): %v", missing, err)
		}
	}
	clean()
	t.Cleanup(clean)

	// Add then read it straight back.
	if err := c.Add(account, service, "sshakku integration test", "pass-one"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if got, found, err := c.Find(account, service); err != nil || !found || got != "pass-one" {
		t.Fatalf("Find after Add = %q, %v, %v; want \"pass-one\", true, nil", got, found, err)
	}

	// List under our account returns exactly the one service we added.
	services, err := c.List(account)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(services) != 1 || services[0] != service {
		t.Fatalf("List = %v, want [%q]", services, service)
	}

	// Adding the same item again is a duplicate the framework rejects — a path
	// the fake, keyed by a Go map, silently overwrites instead.
	if err := c.Add(account, service, "sshakku integration test", "pass-dupe"); err == nil {
		t.Fatal("Add of a duplicate item should fail, got nil")
	}
	// The failed duplicate Add must not have changed the stored value.
	if got, _, err := c.Find(account, service); err != nil || got != "pass-one" {
		t.Fatalf("Find after duplicate Add = %q, %v; want \"pass-one\", nil", got, err)
	}

	// Update overwrites the passphrase in place.
	if err := c.Update(account, service, "pass-two"); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got, found, err := c.Find(account, service); err != nil || !found || got != "pass-two" {
		t.Fatalf("Find after Update = %q, %v, %v; want \"pass-two\", true, nil", got, found, err)
	}

	// Updating an item that was never added is an error, not a silent no-op.
	if err := c.Update(account, missing, "nope"); err == nil {
		t.Fatal("Update of a missing item should fail, got nil")
	}

	// Delete, then confirm the item is gone and a second Delete is a no-op.
	if err := c.Delete(account, service); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, found, err := c.Find(account, service); err != nil || found {
		t.Fatalf("Find after Delete = found=%v, err=%v; want found=false, nil", found, err)
	}
	if err := c.Delete(account, service); err != nil {
		t.Fatalf("second Delete of a missing item should succeed, got %v", err)
	}
	if services, err := c.List(account); err != nil || len(services) != 0 {
		t.Fatalf("List after Delete = %v, %v; want empty, nil", services, err)
	}
}
