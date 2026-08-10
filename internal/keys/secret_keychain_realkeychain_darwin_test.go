//go:build darwin

package keys

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		assert.NoErrorf(t, c.Delete(account, service), "cleanup Delete(%q)", service)
		assert.NoErrorf(t, c.Delete(account, missing), "cleanup Delete(%q)", missing)
	}
	clean()
	t.Cleanup(clean)

	// Add then read it straight back.
	require.NoError(t, c.Add(account, service, "sshakku integration test", "pass-one"),
		"adding an item to the keychain must succeed")
	got, found, err := c.Find(account, service)
	require.NoError(t, err, "reading it straight back must succeed")
	require.True(t, found, "an item just added must be there")
	assert.Equal(t, "pass-one", got, "and hold what was written")

	// List under our account returns exactly the one service we added.
	services, err := c.List(account)
	require.NoError(t, err, "listing this account's items must succeed")
	assert.Equal(t, []string{service}, services, "one item added is one item listed")

	// Adding the same item again is a duplicate the framework rejects — a path
	// the fake, keyed by a Go map, silently overwrites instead.
	assert.Error(t, c.Add(account, service, "sshakku integration test", "pass-dupe"),
		"the framework rejects a duplicate item, and that refusal must be reported")
	// The failed duplicate Add must not have changed the stored value.
	got, _, err = c.Find(account, service)
	require.NoError(t, err, "the item must still be readable after the refused add")
	assert.Equal(t, "pass-one", got, "and hold what it held before: a refused write must change nothing")

	// Update overwrites the passphrase in place.
	require.NoError(t, c.Update(account, service, "pass-two"), "updating an item must succeed")
	got, found, err = c.Find(account, service)
	require.NoError(t, err, "reading the update back must succeed")
	assert.True(t, found, "the item is still there")
	assert.Equal(t, "pass-two", got, "holding the new value, not the one it replaced")

	// Updating an item that was never added is an error, not a silent no-op.
	assert.Error(t, c.Update(account, missing, "nope"),
		"an item that was never added cannot be updated, and saying nothing would report a write that never happened")

	// Delete, then confirm the item is gone and a second Delete is a no-op.
	require.NoError(t, c.Delete(account, service), "removing an item must succeed")
	_, found, err = c.Find(account, service)
	require.NoError(t, err, "looking for a removed item must not be an error")
	assert.False(t, found, "and it must be gone")
	assert.NoError(t, c.Delete(account, service),
		"removing what is already gone is the outcome that was asked for")
	services, err = c.List(account)
	require.NoError(t, err, "listing after the removal must succeed")
	assert.Empty(t, services, "and this account must hold nothing")
}
