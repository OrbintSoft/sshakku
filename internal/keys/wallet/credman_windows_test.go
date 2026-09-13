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

	entry := credential{ //nolint:gosec // G101 matches the Secret field: the value is a made-up one this test writes to a throwaway entry
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

// Every part of an entry that this system cannot spell is refused by the part
// it was, and none of these ever reaches the store: the conversion fails first,
// so nothing is written, read or removed while the name is being worked out.
//
// A key's name is a file name, a label is built from it, and neither is
// something this program chooses the contents of. What must not happen is a
// call made with a name that was quietly cut short at the character the system
// would not take, since every later call would then look somewhere else.
func TestAnEntryThisSystemCannotSpellIsRefusedByThePartItWas(t *testing.T) {
	t.Parallel()

	const refused = "SSHakku-Key-with\x00a-nul"

	t.Run("the name it is filed under", func(t *testing.T) {
		t.Parallel()
		err := credWrite(credential{Target: refused, Secret: "s"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "target name", "which part could not be spelled is the thing to say")
	})

	t.Run("the description beside it", func(t *testing.T) {
		t.Parallel()
		err := credWrite(credential{Target: "SSHakku-Key-fine", Comment: refused, Secret: "s"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "comment")
	})

	t.Run("the account name it carries", func(t *testing.T) {
		t.Parallel()
		err := credWrite(credential{Target: "SSHakku-Key-fine", User: refused, Secret: "s"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "user name")
	})

	t.Run("reading one back", func(t *testing.T) {
		t.Parallel()
		_, found, err := credRead(refused)
		require.Error(t, err)
		assert.False(t, found, "a name that could not be spelled found nothing, and is not a miss either")
	})

	t.Run("removing one", func(t *testing.T) {
		t.Parallel()
		removed, err := credDelete(refused)
		require.Error(t, err)
		assert.False(t, removed, "nothing was removed, and forgetting must not report otherwise")
	})

	t.Run("listing what is there", func(t *testing.T) {
		t.Parallel()
		_, err := credList(refused)
		require.Error(t, err, "an empty answer would read as an account that has saved nothing")
	})
}

// A passphrase over the store's cap is refused where it is written, not only
// where it is encoded. Truncating one would store a secret that is not the
// passphrase, and every later use would fail against a key whose passphrase the
// user knows they saved.
// The name is a throwaway with a cleanup even though nothing should ever be
// written under it: what this test guards against is precisely a write that
// happens when it should not, and a guard that leaves an entry behind the day
// it catches something is not one to run on somebody's own machine.
func TestAPassphraseTooLongForTheStoreStopsTheWrite(t *testing.T) {
	needsRealStore(t)

	target := throwawayTarget(t, "too-long")

	err := credWrite(credential{Target: target, Secret: strings.Repeat("x", maxCredentialBlobSize)})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "too long", "the limit that was met is what the caller is told")

	_, found, readErr := credRead(target)
	require.NoError(t, readErr)
	assert.False(t, found,
		"a passphrase that was refused is not one that was quietly shortened and stored")
}

// What the store refuses for a reason of its own is reported as a failure,
// naming the entry — and told apart from the entry simply not being there,
// which is the ordinary state of a key whose passphrase was never saved.
//
// Nothing is stored by any of these: the store rejects the call, and a name it
// will not take is a name it never files anything under.
func TestWhatTheStoreItselfRefusesIsReportedAndNotReadAsAbsent(t *testing.T) {
	needsRealStore(t)

	// Longer than this system will take for a name, which is how the store is
	// made to refuse without anything being asked of the account's own entries.
	refused := strings.Repeat("x", 40000)

	t.Run("writing", func(t *testing.T) {
		require.Error(t, credWrite(credential{Target: refused, Secret: "s"}))
	})

	t.Run("reading", func(t *testing.T) {
		_, found, err := credRead(refused)
		require.Error(t, err, "a store that refused the question did not answer that there is no entry")
		assert.False(t, found)
	})

	t.Run("removing", func(t *testing.T) {
		removed, err := credDelete(refused)
		require.Error(t, err)
		assert.False(t, removed, "a removal that was refused is not a removal that happened")
	})

	t.Run("listing", func(t *testing.T) {
		_, err := credList(refused)
		require.Error(t, err,
			"an empty list would say this account has saved no passphrases, which is a different thing entirely")
	})
}
