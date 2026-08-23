//go:build windows

package wallet

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// realStore opts a test into reading and writing the account's own credential
// store. It must default to skipped: what these calls touch is the same store
// the rest of the machine keeps its credentials in, and a unit test cannot
// stand in for it. Every entry written carries a target name unique to the run
// and is deleted when the test ends, so nothing is left behind — but a
// developer running the suite on their own machine opts in knowingly, and CI
// sets it because a runner's store is thrown away with the runner.
const realStoreEnv = "SSHAKKU_TEST_ALLOW_REAL_CREDENTIAL_MANAGER"

// needsRealStore skips unless the caller has opted in.
func needsRealStore(t *testing.T) {
	t.Helper()
	if os.Getenv(realStoreEnv) == "" {
		t.Skipf("skipping: set %s=1 to run against this account's own credential store "+
			"(writes only uniquely named entries and deletes them after)", realStoreEnv)
	}
}

// throwawayTarget names an entry no other run can collide with, and arranges
// for it to be gone whatever the test does — including when the test is the
// thing that failed.
func throwawayTarget(t *testing.T, suffix string) string {
	t.Helper()
	target := "SSHakku-Test-" + time.Now().UTC().Format("20060102T150405.000000000") + "-" + suffix
	t.Cleanup(func() { _, _ = credDelete(target) })
	return target
}

// TestASecretIsSpelledTheWayThisSystemSpellsACredentialBlob pins the encoding
// rather than only its symmetry: a blob written in any other spelling round
// trips through this package perfectly well and is mojibake to every other
// program that reads the store.
func TestASecretIsSpelledTheWayThisSystemSpellsACredentialBlob(t *testing.T) {
	t.Parallel()

	blob, err := blobFromSecret("hé")
	require.NoError(t, err)
	assert.Equal(t, []byte{'h', 0x00, 0xe9, 0x00}, blob,
		"a credential blob on this system is UTF-16, little end first, and carries no terminator")
}

// TestASecretSurvivesTheSpellingItIsStoredIn covers what the encoding is for:
// what comes back is what went in, including the characters that need more
// than one code unit.
func TestASecretSurvivesTheSpellingItIsStoredIn(t *testing.T) {
	t.Parallel()

	for _, secret := range []string{"", "plain", "hé", "日本語", "🔑 and a space", "with\ttabs\nand newlines"} {
		blob, err := blobFromSecret(secret)
		require.NoError(t, err, "encoding %q", secret)
		assert.Equal(t, secret, secretFromBlob(blob), "%q did not survive the round trip", secret)
	}
}

// TestASecretTooLongForTheStoreIsRefusedRatherThanTruncated: the store caps a
// blob, and a caller that is over it must be told which limit it met — not
// handed the store's own "invalid parameter", which names nothing.
func TestASecretTooLongForTheStoreIsRefusedRatherThanTruncated(t *testing.T) {
	t.Parallel()

	_, err := blobFromSecret(strings.Repeat("x", maxCredentialBlobSize))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too long", "the error must say what was wrong with it")
}

// TestListingRefusesToAskForEverything: an empty prefix would become the filter
// that matches every credential on the account, which is the one question this
// program must never ask — see F27.
func TestListingRefusesToAskForEverything(t *testing.T) {
	t.Parallel()

	_, err := credList("")
	require.Error(t, err)
}

// TestACredentialWrittenIsReadBackWhole is the round trip against the real
// store: every field written comes back, and the secret with it.
func TestACredentialWrittenIsReadBackWhole(t *testing.T) {
	needsRealStore(t)

	entry := credential{
		Target:  throwawayTarget(t, "whole"),
		Comment: "SSHakku passphrase for a key that does not exist",
		User:    "sshakku",
		Secret:  "corrière della sera",
	}
	require.NoError(t, credWrite(entry))

	got, found, err := credRead(entry.Target)
	require.NoError(t, err)
	require.True(t, found, "the entry just written was not there")
	assert.Equal(t, entry, got)
}

// TestAnEntryWrittenTwiceHoldsTheSecondSecret: the store overwrites in place,
// so nothing above it has to delete before it can store.
func TestAnEntryWrittenTwiceHoldsTheSecondSecret(t *testing.T) {
	needsRealStore(t)

	target := throwawayTarget(t, "twice")
	require.NoError(t, credWrite(credential{Target: target, Secret: "first"}))
	require.NoError(t, credWrite(credential{Target: target, Secret: "second"}))

	got, found, err := credRead(target)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "second", got.Secret)
}

// TestAnEntryThatIsNotThereIsAMissRatherThanAFailure: a key whose passphrase
// was never saved is the ordinary case, and it must not arrive as an error —
// what is above this cannot tell a miss from a broken store otherwise.
func TestAnEntryThatIsNotThereIsAMissRatherThanAFailure(t *testing.T) {
	needsRealStore(t)

	_, found, err := credRead(throwawayTarget(t, "absent"))
	require.NoError(t, err)
	assert.False(t, found)
}

// TestDeletingAnEntryThatIsNotThereSaysSoRatherThanFailing: forgetting an
// already-forgotten key is not a failure, and the caller is still told which of
// the two happened.
func TestDeletingAnEntryThatIsNotThereSaysSoRatherThanFailing(t *testing.T) {
	needsRealStore(t)

	existed, err := credDelete(throwawayTarget(t, "delete-absent"))
	require.NoError(t, err)
	assert.False(t, existed)
}

// TestAnEntryIsGoneAfterItIsDeleted, and the store says it was there to go.
func TestAnEntryIsGoneAfterItIsDeleted(t *testing.T) {
	needsRealStore(t)

	target := throwawayTarget(t, "delete")
	require.NoError(t, credWrite(credential{Target: target, Secret: "s"}))

	existed, err := credDelete(target)
	require.NoError(t, err)
	assert.True(t, existed)

	_, found, err := credRead(target)
	require.NoError(t, err)
	assert.False(t, found, "the entry is still there after being deleted")
}

// TestOnlyTheEntriesUnderThePrefixAreListed is F27 at this level: the store is
// shared with every other program on the machine, and the question asked of it
// is narrow enough that nobody else's entry is ever in the answer.
func TestOnlyTheEntriesUnderThePrefixAreListed(t *testing.T) {
	needsRealStore(t)

	prefix := "SSHakku-Test-" + time.Now().UTC().Format("20060102T150405.000000000") + "-listed"
	mine := []string{prefix + "-one", prefix + "-two"}
	for _, target := range mine {
		require.NoError(t, credWrite(credential{Target: target, Secret: "s"}))
		t.Cleanup(func() { _, _ = credDelete(target) })
	}
	stranger := prefix + "X-not-mine"
	require.NoError(t, credWrite(credential{Target: stranger, Secret: "s"}))
	t.Cleanup(func() { _, _ = credDelete(stranger) })

	listed, err := credList(prefix + "-")
	require.NoError(t, err)
	assert.ElementsMatch(t, mine, listed)
}

// TestListingAPrefixNothingWasStoredUnderIsEmptyRatherThanAnError: an account
// that has saved no passphrase yet is the state every account starts in.
func TestListingAPrefixNothingWasStoredUnderIsEmptyRatherThanAnError(t *testing.T) {
	needsRealStore(t)

	listed, err := credList(throwawayTarget(t, "empty") + "-")
	require.NoError(t, err)
	assert.Empty(t, listed)
}
