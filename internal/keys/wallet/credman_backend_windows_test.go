//go:build windows

package wallet

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/run"
)

// CredentialManager is a Backend, and nothing else in this package has to be
// asked whether it still is.
var _ Backend = (*CredentialManager)(nil)

// fakeCredentialStore stands in for the account's own store. It replaces the
// system calls — which no test can stand in for and which credman_windows_test
// exercises for real — and never any decision made above them.
type fakeCredentialStore struct {
	entries map[string]credential
	// asked records the prefix List was given, which is the whole of how this
	// backend keeps to its own entries.
	asked string
	// answers is what List hands back, whatever was written.
	answers []string
	// blocked, when non-nil, holds every call until it is closed.
	blocked chan struct{}
	err     error
}

func newFakeCredentialStore() *fakeCredentialStore {
	return &fakeCredentialStore{entries: map[string]credential{}}
}

func (f *fakeCredentialStore) wait() {
	if f.blocked != nil {
		<-f.blocked
	}
}

func (f *fakeCredentialStore) Read(target string) (credential, bool, error) {
	f.wait()
	if f.err != nil {
		return credential{}, false, f.err
	}
	entry, found := f.entries[target]
	return entry, found, nil
}

func (f *fakeCredentialStore) Write(entry credential) error {
	f.wait()
	if f.err != nil {
		return f.err
	}
	f.entries[entry.Target] = entry
	return nil
}

func (f *fakeCredentialStore) Delete(target string) (bool, error) {
	f.wait()
	if f.err != nil {
		return false, f.err
	}
	_, found := f.entries[target]
	delete(f.entries, target)
	return found, nil
}

func (f *fakeCredentialStore) List(prefix string) ([]string, error) {
	f.wait()
	f.asked = prefix
	if f.err != nil {
		return nil, f.err
	}
	return f.answers, nil
}

// backedBy builds a backend over a fake store.
func backedBy(store credentialStore, prefix string) *CredentialManager {
	return &CredentialManager{ServicePrefix: prefix, credentials: store}
}

// TestAPassphraseIsFiledUnderTheServiceItWasStoredFor: the service identifier
// is the entry's name, so what one shell wrote is what the next one asks for.
func TestAPassphraseIsFiledUnderTheServiceItWasStoredFor(t *testing.T) {
	t.Parallel()

	store := newFakeCredentialStore()
	backend := backedBy(store, "")
	require.NoError(t, backend.Store(t.Context(), "SSHakku-Key-id_ed25519", "SSH Passphrase for id_ed25519", "hunter2"))

	entry := store.entries["SSHakku-Key-id_ed25519"]
	assert.Equal(t, "SSHakku-Key-id_ed25519", entry.Target)
	assert.Equal(t, "SSH Passphrase for id_ed25519", entry.Comment)
	assert.Equal(t, "hunter2", entry.Secret)
}

// TestAnEntryNamesWhoPutItThere: the store's own tooling shows a user name
// beside the target, so an entry says where it came from where a person is
// looking at it rather than only in a name they would have to know how to read.
func TestAnEntryNamesWhoPutItThere(t *testing.T) {
	t.Parallel()

	for name, prefix := range map[string]string{"the default name": "", "a name of your own": "work"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			store := newFakeCredentialStore()
			backend := backedBy(store, prefix)
			require.NoError(t, backend.Store(t.Context(), "svc", "label", "s"))

			assert.Equal(t, ServicePrefixOrDefault(prefix), store.entries["svc"].User)
		})
	}
}

// TestALookupFindsWhatWasStored, which is the whole of what a later shell does.
func TestALookupFindsWhatWasStored(t *testing.T) {
	t.Parallel()

	store := newFakeCredentialStore()
	backend := backedBy(store, "")
	require.NoError(t, backend.Store(t.Context(), "svc", "label", "hunter2"))

	got, found, err := backend.Lookup(t.Context(), "svc")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "hunter2", got)
}

// TestALookupForAKeyNothingWasStoredForIsAMiss: a key whose passphrase was
// never saved is the ordinary case. Reported as an error, it would send the
// loader off to say something had gone wrong with the wallet.
func TestALookupForAKeyNothingWasStoredForIsAMiss(t *testing.T) {
	t.Parallel()

	got, found, err := backedBy(newFakeCredentialStore(), "").Lookup(t.Context(), "svc")
	require.NoError(t, err)
	assert.False(t, found)
	assert.Empty(t, got)
}

// TestForgettingAnEntryThatIsNotThereIsNotAFailure: forgetting a key twice is
// the same as forgetting it once.
func TestForgettingAnEntryThatIsNotThereIsNotAFailure(t *testing.T) {
	t.Parallel()

	assert.NoError(t, backedBy(newFakeCredentialStore(), "").Delete(t.Context(), "svc"))
}

// TestListingAsksOnlyForThisConfigurationsOwnEntries is F27 as this backend
// keeps it: the narrowing is in the question, so the question is what the test
// looks at. A prefix of the user's own moves the whole of it.
func TestListingAsksOnlyForThisConfigurationsOwnEntries(t *testing.T) {
	t.Parallel()

	for name, testCase := range map[string]struct{ prefix, asked string }{
		"the default name":   {prefix: "", asked: "SSHakku-Key-"},
		"a name of your own": {prefix: "work", asked: "work-"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			store := newFakeCredentialStore()
			_, err := backedBy(store, testCase.prefix).List(t.Context())
			require.NoError(t, err)
			assert.Equal(t, testCase.asked, store.asked)
		})
	}
}

// TestTheStoresAnswerIsNotNarrowedASecondTime pins the other half of that: the
// answer is taken whole. A filter here would go on passing if the query ever
// stopped being narrow, which is the one failure it would need to reveal.
func TestTheStoresAnswerIsNotNarrowedASecondTime(t *testing.T) {
	t.Parallel()

	store := newFakeCredentialStore()
	store.answers = []string{"SSHakku-Key-id_ed25519", "something-nobody-here-would-have-written"}

	got, err := backedBy(store, "").List(t.Context())
	require.NoError(t, err)
	assert.Equal(t, store.answers, got)
}

// TestAStoreThatWillNotAnswerBecomesAnErrorRatherThanAnEndlessWait is F21 at
// this seam: a login shell and an ssh at a passphrase prompt are both behind
// this call, and one that neither answers nor fails must become something the
// caller can fall back from.
func TestAStoreThatWillNotAnswerBecomesAnErrorRatherThanAnEndlessWait(t *testing.T) {
	t.Parallel()

	store := newFakeCredentialStore()
	store.blocked = make(chan struct{})
	// Released whatever the test does, so the call it is holding can finish
	// and its goroutine end with the run rather than outlive it.
	t.Cleanup(func() { close(store.blocked) })

	backend := backedBy(store, "")
	backend.Timeout = 10 * time.Millisecond

	_, _, err := backend.Lookup(t.Context(), "svc")
	require.ErrorIs(t, err, run.ErrTimedOut)
}

// TestAStoreThatFailsSaysSoRatherThanLookingEmpty: a store that could not be
// read is not a store with nothing in it, and forgetting everything must not
// report success having listed nothing.
func TestAStoreThatFailsSaysSoRatherThanLookingEmpty(t *testing.T) {
	t.Parallel()

	store := newFakeCredentialStore()
	store.err = errors.New("the store said no")

	_, err := backedBy(store, "").List(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "the store said no")
}
