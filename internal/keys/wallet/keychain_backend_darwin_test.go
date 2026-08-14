//go:build darwin

package wallet

import (
	"errors"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeKeychainClient is an in-memory KeychainClient for Keychain's
// unit tests, keyed by service (Account is recorded but not used to key
// storage — every test uses a single fixed account).
type fakeKeychainClient struct {
	items                                             map[string]string
	addErr, updateErr, findErr, deleteErr, listErr    error
	addCalls, updateCalls                             int
	lastAccount, lastService, lastLabel, lastPassword string
}

func (f *fakeKeychainClient) Add(account, service, label, passphrase string) error {
	f.addCalls++
	f.lastAccount, f.lastService, f.lastLabel, f.lastPassword = account, service, label, passphrase
	if f.addErr != nil {
		return f.addErr
	}
	if f.items == nil {
		f.items = map[string]string{}
	}
	f.items[service] = passphrase
	return nil
}

func (f *fakeKeychainClient) Update(account, service, passphrase string) error {
	f.updateCalls++
	f.lastAccount, f.lastService, f.lastPassword = account, service, passphrase
	if f.updateErr != nil {
		return f.updateErr
	}
	f.items[service] = passphrase
	return nil
}

func (f *fakeKeychainClient) Find(account, service string) (string, bool, error) {
	if f.findErr != nil {
		return "", false, f.findErr
	}
	p, ok := f.items[service]
	return p, ok, nil
}

func (f *fakeKeychainClient) Delete(account, service string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	delete(f.items, service)
	return nil
}

func (f *fakeKeychainClient) List(account string) ([]string, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	services := make([]string, 0, len(f.items))
	for s := range f.items {
		services = append(services, s)
	}
	sort.Strings(services)
	return services, nil
}

var _ KeychainClient = (*fakeKeychainClient)(nil)

func TestKeychainBackendLookup(t *testing.T) {
	t.Run("hit", func(t *testing.T) {
		c := &fakeKeychainClient{items: map[string]string{"svc": "hunter2"}}
		b := &Keychain{Client: c, Account: "alice"}
		got, found, err := b.Lookup(t.Context(), "svc")
		require.NoError(t, err, "a stored passphrase must come back")
		assert.True(t, found, "the item is in the keychain, so it must be reported found")
		assert.Equal(t, "hunter2", got, "and the passphrase read out must be the one that was stored")
	})
	t.Run("miss", func(t *testing.T) {
		c := &fakeKeychainClient{}
		b := &Keychain{Client: c, Account: "alice"}
		_, found, err := b.Lookup(t.Context(), "svc")
		require.NoError(t, err, "a passphrase that was never stored is not an error")
		assert.False(t, found, "and nothing may be reported found")
	})
	t.Run("client error", func(t *testing.T) {
		wantErr := errors.New("boom")
		c := &fakeKeychainClient{findErr: wantErr}
		b := &Keychain{Client: c, Account: "alice"}
		_, _, err := b.Lookup(t.Context(), "svc")
		assert.ErrorIs(t, err, wantErr, "a keychain that could not be read must be reported, not read as a miss")
	})
}

func TestKeychainBackendStore(t *testing.T) {
	t.Run("new item calls Add, not Update", func(t *testing.T) {
		c := &fakeKeychainClient{}
		b := &Keychain{Client: c, Account: "alice"}
		require.NoError(t, b.Store(t.Context(), "svc", "label", "hunter2"), "saving a passphrase must succeed")
		assert.Equal(t, 1, c.addCalls, "an item that is not there yet must be added")
		assert.Zero(t, c.updateCalls, "and nothing updated: there is nothing to update")
		assert.Equal(t, "alice", c.lastAccount, "the item must be filed under this user's account")
		assert.Equal(t, "svc", c.lastService, "named after the key it belongs to")
		assert.Equal(t, "label", c.lastLabel, "labelled with what a person sees in Keychain Access")
		assert.Equal(t, "hunter2", c.lastPassword, "and holding the passphrase that was typed")
	})
	t.Run("existing item calls Update, not Add", func(t *testing.T) {
		c := &fakeKeychainClient{items: map[string]string{"svc": "old"}}
		b := &Keychain{Client: c, Account: "alice"}
		require.NoError(t, b.Store(t.Context(), "svc", "label", "new"), "replacing a passphrase must succeed")
		assert.Zero(t, c.addCalls, "adding again would leave a second copy of the secret in the keychain")
		assert.Equal(t, 1, c.updateCalls, "so an item that is there must be updated in place")
		assert.Equal(t, "new", c.items["svc"], "and hold the passphrase that replaced the old one")
	})
	t.Run("find error", func(t *testing.T) {
		wantErr := errors.New("boom")
		c := &fakeKeychainClient{findErr: wantErr}
		b := &Keychain{Client: c, Account: "alice"}
		assert.ErrorIs(t, b.Store(t.Context(), "svc", "label", "x"), wantErr,
			"a keychain that could not be read must be reported: writing blind could overwrite the wrong item")
	})
	t.Run("add error", func(t *testing.T) {
		wantErr := errors.New("boom")
		c := &fakeKeychainClient{addErr: wantErr}
		b := &Keychain{Client: c, Account: "alice"}
		assert.ErrorIs(t, b.Store(t.Context(), "svc", "label", "x"), wantErr,
			"a passphrase the keychain refused to add must not be reported as saved")
	})
	t.Run("update error", func(t *testing.T) {
		wantErr := errors.New("boom")
		c := &fakeKeychainClient{items: map[string]string{"svc": "old"}, updateErr: wantErr}
		b := &Keychain{Client: c, Account: "alice"}
		assert.ErrorIs(t, b.Store(t.Context(), "svc", "label", "x"), wantErr,
			"a passphrase the keychain refused to replace must not be reported as saved")
	})
}

func TestKeychainBackendDelete(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		c := &fakeKeychainClient{items: map[string]string{"svc": "x"}}
		b := &Keychain{Client: c, Account: "alice"}
		require.NoError(t, b.Delete(t.Context(), "svc"), "forgetting a passphrase must succeed")
		assert.NotContains(t, c.items, "svc", "and the item must actually be gone from the keychain")
	})
	t.Run("missing entry is not an error", func(t *testing.T) {
		c := &fakeKeychainClient{}
		b := &Keychain{Client: c, Account: "alice"}
		require.NoError(t, b.Delete(t.Context(), "svc"),
			"a passphrase that is already not there is the outcome that was asked for")
	})
	t.Run("client error", func(t *testing.T) {
		wantErr := errors.New("boom")
		c := &fakeKeychainClient{deleteErr: wantErr}
		b := &Keychain{Client: c, Account: "alice"}
		assert.ErrorIs(t, b.Delete(t.Context(), "svc"), wantErr,
			"a passphrase the keychain refused to remove must not be reported as forgotten")
	})
}

// TestKeychainBackendListLeavesOtherItemsAlone verifies F27 for the state a
// real login keychain is in: every application that saves a password puts it
// there, under the same account, so a query that names only the account
// answers with everyone's secrets. Whatever List reports is what
// `forget --all` goes on to delete.
func TestKeychainBackendListLeavesOtherItemsAlone(t *testing.T) {
	c := &fakeKeychainClient{items: map[string]string{
		defaultServicePrefix + "-id_ed25519": "x",
		"Another App-credentials":            "y",
		"AirPort network password":           "z",
		defaultServicePrefix + "-id_rsa":     "w",
	}}
	b := &Keychain{Client: c, Account: "alice"}
	got, err := b.List(t.Context())
	require.NoError(t, err, "listing what SSHakku keeps in the keychain must succeed")
	assert.Equal(t, []string{defaultServicePrefix + "-id_ed25519", defaultServicePrefix + "-id_rsa"}, got,
		"only SSHakku's own items may be reported: the login keychain holds every application's passwords, "+
			"and whatever is listed here is what forget --all goes on to delete")
}

// TestKeychainBackendListGoesByTheBackendsOwnPrefix verifies F32 in the store
// that makes it matter most: the login keychain holds everyone's passwords, so
// the prefix is the whole of what marks an item as SSHakku's. Under a
// configured prefix an item carrying the default one was written by something
// else, and `forget --all` — which deletes exactly what List reports — must not
// reach it.
func TestKeychainBackendListGoesByTheBackendsOwnPrefix(t *testing.T) {
	const mine = "wallet-of-mine"
	c := &fakeKeychainClient{items: map[string]string{
		mine + "-id_ed25519":             "x",
		defaultServicePrefix + "-id_rsa": "y",
		"Another App-credentials":        "z",
	}}
	b := &Keychain{Client: c, Account: "alice", ServicePrefix: mine}
	got, err := b.List(t.Context())
	require.NoError(t, err, "listing what SSHakku keeps in the keychain must succeed")
	assert.Equal(t, []string{mine + "-id_ed25519"}, got,
		"under a configured prefix, an item carrying the default one was written by something else and must be left alone")
}

func TestKeychainBackendList(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		c := &fakeKeychainClient{items: map[string]string{defaultServicePrefix + "-b": "x", defaultServicePrefix + "-a": "y"}}
		b := &Keychain{Client: c, Account: "alice"}
		got, err := b.List(t.Context())
		require.NoError(t, err, "listing what the keychain holds must succeed")
		assert.Equal(t, []string{defaultServicePrefix + "-a", defaultServicePrefix + "-b"}, got,
			"every item must be named, by the key it belongs to")
	})
	t.Run("client error", func(t *testing.T) {
		wantErr := errors.New("boom")
		c := &fakeKeychainClient{listErr: wantErr}
		b := &Keychain{Client: c, Account: "alice"}
		_, err := b.List(t.Context())
		assert.ErrorIs(t, err, wantErr, "a keychain that could not be read must not be listed as empty")
	})
}
