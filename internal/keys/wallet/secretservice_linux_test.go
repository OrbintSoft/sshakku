package wallet

import (
	"context"
	"errors"
	"testing"

	"github.com/godbus/dbus/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSecretServiceClient scripts SecretServiceClient for SecretService
// tests, recording the objects passed to Unlock/Lock so tests can assert the
// unlock/lock bracket around a Lookup/Store.
type fakeSecretServiceClient struct {
	collection      dbus.ObjectPath
	collectionErr   error
	collectionCalls int
	// collectionAsked records the (alias, label) pair each resolution asked
	// for, so a test can read which collection was addressed rather than
	// which one the backend meant to address.
	collectionAsked []ssCollectionCall

	unlockErr error
	unlocked  []dbus.ObjectPath

	lockErr error
	locked  []dbus.ObjectPath

	items         []dbus.ObjectPath
	searchErr     error
	searchedAttrs map[string]string

	secretsByItem map[dbus.ObjectPath]string
	secretErr     error

	createErr    error
	createdItems []ssCreateCall

	itemsByCollection map[dbus.ObjectPath][]dbus.ObjectPath
	itemsErr          error

	attrsByItem map[dbus.ObjectPath]map[string]string
	attrsErr    error

	deleteItemErr error
	deletedItems  []dbus.ObjectPath
}

type ssCollectionCall struct {
	alias string
	label string
}

type ssCreateCall struct {
	collection dbus.ObjectPath
	label      string
	attrs      map[string]string
	passphrase string
	replace    bool
}

func (f *fakeSecretServiceClient) Collection(_ context.Context, alias, label string) (dbus.ObjectPath, error) {
	f.collectionCalls++
	f.collectionAsked = append(f.collectionAsked, ssCollectionCall{alias: alias, label: label})
	if f.collectionErr != nil {
		return "", f.collectionErr
	}
	return f.collection, nil
}

func (f *fakeSecretServiceClient) Unlock(_ context.Context, objects ...dbus.ObjectPath) error {
	f.unlocked = append(f.unlocked, objects...)
	return f.unlockErr
}

func (f *fakeSecretServiceClient) Lock(_ context.Context, objects ...dbus.ObjectPath) error {
	f.locked = append(f.locked, objects...)
	return f.lockErr
}

func (f *fakeSecretServiceClient) SearchItems(_ context.Context, _ dbus.ObjectPath, attrs map[string]string) ([]dbus.ObjectPath, error) {
	f.searchedAttrs = attrs
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	return f.items, nil
}

func (f *fakeSecretServiceClient) GetSecret(_ context.Context, item dbus.ObjectPath) (string, error) {
	if f.secretErr != nil {
		return "", f.secretErr
	}
	return f.secretsByItem[item], nil
}

func (f *fakeSecretServiceClient) CreateItem(_ context.Context, collection dbus.ObjectPath, label string, attrs map[string]string, passphrase string, replace bool) error {
	f.createdItems = append(f.createdItems, ssCreateCall{collection, label, attrs, passphrase, replace})
	return f.createErr
}

func (f *fakeSecretServiceClient) Items(_ context.Context, collection dbus.ObjectPath) ([]dbus.ObjectPath, error) {
	if f.itemsErr != nil {
		return nil, f.itemsErr
	}
	return f.itemsByCollection[collection], nil
}

func (f *fakeSecretServiceClient) ItemAttributes(_ context.Context, item dbus.ObjectPath) (map[string]string, error) {
	if f.attrsErr != nil {
		return nil, f.attrsErr
	}
	return f.attrsByItem[item], nil
}

func (f *fakeSecretServiceClient) DeleteItem(_ context.Context, item dbus.ObjectPath) error {
	if f.deleteItemErr != nil {
		return f.deleteItemErr
	}
	f.deletedItems = append(f.deletedItems, item)
	return nil
}

func TestSecretServiceLookup(t *testing.T) {
	const col = dbus.ObjectPath("/org/freedesktop/secrets/collection/sshakku")
	const item = dbus.ObjectPath("/org/freedesktop/secrets/collection/sshakku/1")

	t.Run("hit unlocks, reads the secret, and locks again", func(t *testing.T) {
		c := &fakeSecretServiceClient{
			collection:    col,
			items:         []dbus.ObjectPath{item},
			secretsByItem: map[dbus.ObjectPath]string{item: "hunter2"},
		}
		b := &SecretService{Client: c, User: "alice"}

		pass, found, err := b.Lookup(t.Context(), defaultServicePrefix+"-id_rsa")
		require.NoError(t, err, "a stored passphrase must come back")
		assert.True(t, found, "the item is there, so it must be reported found")
		assert.Equal(t, "hunter2", pass, "and the passphrase read out must be the one that was stored")
		assert.Equal(t, []dbus.ObjectPath{col}, c.unlocked, "the collection holding the item is what must be unlocked")
		assert.Equal(t, []dbus.ObjectPath{col}, c.locked, "and it must be locked again afterwards")
		assert.Equal(t, map[string]string{"service": defaultServicePrefix + "-id_rsa", "username": "alice"}, c.searchedAttrs,
			"the search must name both the key and whose passphrase it is")
	})

	t.Run("miss is found=false, no error, still locks", func(t *testing.T) {
		c := &fakeSecretServiceClient{collection: col}
		b := &SecretService{Client: c, User: "alice"}

		_, found, err := b.Lookup(t.Context(), defaultServicePrefix+"-id_rsa")
		require.NoError(t, err, "a passphrase that was never stored is not an error")
		assert.False(t, found, "and nothing may be reported found")
		assert.Len(t, c.locked, 1, "the collection must be locked again even when nothing was there")
	})

	t.Run("the collection is resolved once and cached", func(t *testing.T) {
		c := &fakeSecretServiceClient{collection: col}
		b := &SecretService{Client: c, User: "alice"}

		_, _, err := b.Lookup(t.Context(), "a")
		require.NoError(t, err, "the first lookup must succeed")
		_, _, err = b.Lookup(t.Context(), "b")
		require.NoError(t, err, "and so must the second")
		assert.Equal(t, 1, c.collectionCalls, "the collection is the same one; asking the bus for it twice is a round trip nobody needs")
	})

	t.Run("a collection error is returned, nothing is unlocked", func(t *testing.T) {
		wantErr := errors.New("boom")
		c := &fakeSecretServiceClient{collectionErr: wantErr}
		b := &SecretService{Client: c, User: "alice"}

		_, _, err := b.Lookup(t.Context(), "x")
		assert.ErrorIs(t, err, wantErr, "a collection that could not be resolved must be reported as it was refused")
		assert.Empty(t, c.unlocked, "and nothing may be unlocked on the strength of a collection nobody found")
	})

	t.Run("an unlock error is returned, the collection is not locked", func(t *testing.T) {
		wantErr := errors.New("dismissed")
		c := &fakeSecretServiceClient{collection: col, unlockErr: wantErr}
		b := &SecretService{Client: c, User: "alice"}

		_, _, err := b.Lookup(t.Context(), "x")
		assert.ErrorIs(t, err, wantErr, "a user who dismissed the unlock dialog must be told so")
		assert.Empty(t, c.locked, "and a collection that never opened has nothing to re-lock")
	})

	t.Run("a search error is returned, the collection is still locked", func(t *testing.T) {
		wantErr := errors.New("boom")
		c := &fakeSecretServiceClient{collection: col, searchErr: wantErr}
		b := &SecretService{Client: c, User: "alice"}

		_, _, err := b.Lookup(t.Context(), "x")
		assert.ErrorIs(t, err, wantErr, "a search that failed must be reported, not read as a miss")
		assert.Len(t, c.locked, 1, "and the collection this opened must not be left open behind the error")
	})

	t.Run("a get-secret error is returned, the collection is still locked", func(t *testing.T) {
		wantErr := errors.New("boom")
		c := &fakeSecretServiceClient{collection: col, items: []dbus.ObjectPath{item}, secretErr: wantErr}
		b := &SecretService{Client: c, User: "alice"}

		_, found, err := b.Lookup(t.Context(), "x")
		assert.ErrorIs(t, err, wantErr, "a secret that could not be read must be reported as unread")
		assert.False(t, found, "an item nobody could read out is not a passphrase found")
		assert.Len(t, c.locked, 1, "and the collection this opened must not be left open behind the error")
	})
}

func TestSecretServiceSession(t *testing.T) {
	const col = dbus.ObjectPath("/org/freedesktop/secrets/collection/sshakku")
	const item = dbus.ObjectPath("/org/freedesktop/secrets/collection/sshakku/1")

	t.Run("Unlock then several Lookup/Store share one unlock, Lock releases it", func(t *testing.T) {
		c := &fakeSecretServiceClient{
			collection:    col,
			items:         []dbus.ObjectPath{item},
			secretsByItem: map[dbus.ObjectPath]string{item: "hunter2"},
		}
		b := &SecretService{Client: c, User: "alice"}

		require.NoError(t, b.Unlock(t.Context()), "opening the collection for a run of work must succeed")
		_, _, err := b.Lookup(t.Context(), "a")
		require.NoError(t, err, "a lookup inside the session must succeed")
		require.NoError(t, b.Store(t.Context(), "b", "label", "hunter2"), "and so must a store")
		assert.Len(t, c.unlocked, 1,
			"one unlock covers the whole session; unlocking per call is a wallet dialog per key")
		assert.Empty(t, c.locked, "and nothing may be locked while the session is still open")

		require.NoError(t, b.Lock(t.Context()), "closing the session must succeed")
		assert.Len(t, c.locked, 1, "and it must lock the collection exactly once")

		_, _, err = b.Lookup(t.Context(), "c")
		require.NoError(t, err, "work after the session must still succeed")
		assert.Len(t, c.unlocked, 2, "a call outside a session opens the collection itself")
		assert.Len(t, c.locked, 2, "and closes it again")
	})

	t.Run("an Unlock collection error leaves held false", func(t *testing.T) {
		wantErr := errors.New("boom")
		c := &fakeSecretServiceClient{collectionErr: wantErr}
		b := &SecretService{Client: c, User: "alice"}

		assert.ErrorIs(t, b.Unlock(t.Context()), wantErr, "a session that could not be opened must say so")
		assert.False(t, b.held, "and must not believe it holds a collection it never opened")
	})
}

func TestSecretServiceStore(t *testing.T) {
	const col = dbus.ObjectPath("/org/freedesktop/secrets/collection/sshakku")

	t.Run("unlocks, creates the item, and locks again", func(t *testing.T) {
		c := &fakeSecretServiceClient{collection: col}
		b := &SecretService{Client: c, User: "alice"}

		require.NoError(t, b.Store(t.Context(), defaultServicePrefix+"-id_rsa", "SSH Passphrase for id_rsa", "hunter2"),
			"saving a passphrase must succeed")
		assert.Equal(t, []dbus.ObjectPath{col}, c.unlocked, "the collection to write into is what must be unlocked")
		assert.Equal(t, []dbus.ObjectPath{col}, c.locked, "and it must be locked again afterwards")
		require.Len(t, c.createdItems, 1, "one passphrase saved is one item written")
		assert.Equal(t, ssCreateCall{
			collection: col,
			label:      "SSH Passphrase for id_rsa",
			attrs:      map[string]string{"service": defaultServicePrefix + "-id_rsa", "username": "alice"},
			passphrase: "hunter2",
			replace:    true,
		}, c.createdItems[0],
			"the item must name the key, say whose it is, carry the passphrase, and replace any earlier one")
	})

	t.Run("a collection error is returned, nothing is unlocked", func(t *testing.T) {
		wantErr := errors.New("boom")
		c := &fakeSecretServiceClient{collectionErr: wantErr}
		b := &SecretService{Client: c, User: "alice"}

		assert.ErrorIs(t, b.Store(t.Context(), "x", "y", "z"), wantErr,
			"a collection that could not be resolved must be reported as it was refused")
		assert.Empty(t, c.unlocked, "and nothing may be unlocked on the strength of a collection nobody found")
	})

	t.Run("an unlock error is returned, the collection is not locked", func(t *testing.T) {
		wantErr := errors.New("dismissed")
		c := &fakeSecretServiceClient{collection: col, unlockErr: wantErr}
		b := &SecretService{Client: c, User: "alice"}

		assert.ErrorIs(t, b.Store(t.Context(), "x", "y", "z"), wantErr, "a user who dismissed the unlock dialog must be told so")
		assert.Empty(t, c.locked, "and a collection that never opened has nothing to re-lock")
	})

	t.Run("a create-item error is returned, the collection is still locked", func(t *testing.T) {
		wantErr := errors.New("boom")
		c := &fakeSecretServiceClient{collection: col, createErr: wantErr}
		b := &SecretService{Client: c, User: "alice"}

		assert.ErrorIs(t, b.Store(t.Context(), "x", "y", "z"), wantErr,
			"a passphrase the wallet refused to write must not be reported as saved")
		assert.Len(t, c.locked, 1, "and the collection this opened must not be left open behind the error")
	})
}

func TestSecretServiceDelete(t *testing.T) {
	const col = dbus.ObjectPath("/org/freedesktop/secrets/collection/sshakku")
	const item = dbus.ObjectPath("/org/freedesktop/secrets/collection/sshakku/1")

	t.Run("hit unlocks, deletes the matching item, and locks again", func(t *testing.T) {
		c := &fakeSecretServiceClient{collection: col, items: []dbus.ObjectPath{item}}
		b := &SecretService{Client: c, User: "alice"}

		require.NoError(t, b.Delete(t.Context(), defaultServicePrefix+"-id_rsa"), "forgetting a passphrase must succeed")
		assert.Equal(t, []dbus.ObjectPath{col}, c.unlocked, "the collection holding the item is what must be unlocked")
		assert.Equal(t, []dbus.ObjectPath{col}, c.locked, "and it must be locked again afterwards")
		assert.Equal(t, []dbus.ObjectPath{item}, c.deletedItems,
			"exactly the item that was asked about is the one that may be removed")
	})

	t.Run("a miss is success, no error, still locks, nothing deleted", func(t *testing.T) {
		c := &fakeSecretServiceClient{collection: col}
		b := &SecretService{Client: c, User: "alice"}

		require.NoError(t, b.Delete(t.Context(), defaultServicePrefix+"-id_rsa"),
			"a passphrase that is already not there is the outcome that was asked for")
		assert.Len(t, c.locked, 1, "the collection must be locked again even when nothing was there")
		assert.Empty(t, c.deletedItems, "and nothing may be removed when nothing matched")
	})

	t.Run("a collection error is returned, nothing is unlocked", func(t *testing.T) {
		wantErr := errors.New("boom")
		c := &fakeSecretServiceClient{collectionErr: wantErr}
		b := &SecretService{Client: c, User: "alice"}

		assert.ErrorIs(t, b.Delete(t.Context(), "x"), wantErr,
			"a collection that could not be resolved must be reported as it was refused")
		assert.Empty(t, c.unlocked, "and nothing may be unlocked on the strength of a collection nobody found")
	})

	t.Run("an unlock error is returned, the collection is not locked", func(t *testing.T) {
		wantErr := errors.New("dismissed")
		c := &fakeSecretServiceClient{collection: col, unlockErr: wantErr}
		b := &SecretService{Client: c, User: "alice"}

		assert.ErrorIs(t, b.Delete(t.Context(), "x"), wantErr, "a user who dismissed the unlock dialog must be told so")
		assert.Empty(t, c.locked, "and a collection that never opened has nothing to re-lock")
	})

	t.Run("a search error is returned, the collection is still locked", func(t *testing.T) {
		wantErr := errors.New("boom")
		c := &fakeSecretServiceClient{collection: col, searchErr: wantErr}
		b := &SecretService{Client: c, User: "alice"}

		assert.ErrorIs(t, b.Delete(t.Context(), "x"), wantErr, "a search that failed must be reported, not read as nothing to remove")
		assert.Len(t, c.locked, 1, "and the collection this opened must not be left open behind the error")
	})

	t.Run("a delete-item error is returned, the collection is still locked", func(t *testing.T) {
		wantErr := errors.New("boom")
		c := &fakeSecretServiceClient{collection: col, items: []dbus.ObjectPath{item}, deleteItemErr: wantErr}
		b := &SecretService{Client: c, User: "alice"}

		assert.ErrorIs(t, b.Delete(t.Context(), "x"), wantErr,
			"a passphrase the wallet refused to remove must not be reported as forgotten")
		assert.Len(t, c.locked, 1, "and the collection this opened must not be left open behind the error")
	})
}

func TestSecretServiceList(t *testing.T) {
	const col = dbus.ObjectPath("/org/freedesktop/secrets/collection/sshakku")
	const item1 = dbus.ObjectPath("/org/freedesktop/secrets/collection/sshakku/1")
	const item2 = dbus.ObjectPath("/org/freedesktop/secrets/collection/sshakku/2")

	t.Run("returns the service attribute of every item, unlocks, and locks again", func(t *testing.T) {
		c := &fakeSecretServiceClient{
			collection:        col,
			itemsByCollection: map[dbus.ObjectPath][]dbus.ObjectPath{col: {item1, item2}},
			attrsByItem: map[dbus.ObjectPath]map[string]string{
				item1: {"service": defaultServicePrefix + "-id_rsa", "username": "alice"},
				item2: {"service": defaultServicePrefix + "-id_ed25519", "username": "alice"},
			},
		}
		b := &SecretService{Client: c, User: "alice"}

		got, err := b.List(t.Context())
		require.NoError(t, err, "listing what the wallet holds must succeed")
		assert.Equal(t, []string{defaultServicePrefix + "-id_rsa", defaultServicePrefix + "-id_ed25519"}, got,
			"every item the collection holds must be named, by the key it belongs to")
		assert.Equal(t, []dbus.ObjectPath{col}, c.unlocked, "the collection being listed is what must be unlocked")
		assert.Equal(t, []dbus.ObjectPath{col}, c.locked, "and it must be locked again afterwards")
	})

	t.Run("an empty collection returns none, no error", func(t *testing.T) {
		c := &fakeSecretServiceClient{collection: col}
		b := &SecretService{Client: c, User: "alice"}

		got, err := b.List(t.Context())
		require.NoError(t, err, "a wallet holding nothing is not an error")
		assert.Empty(t, got, "and nothing may be listed")
	})

	t.Run("a collection error is returned, nothing is unlocked", func(t *testing.T) {
		wantErr := errors.New("boom")
		c := &fakeSecretServiceClient{collectionErr: wantErr}
		b := &SecretService{Client: c, User: "alice"}

		_, err := b.List(t.Context())
		assert.ErrorIs(t, err, wantErr, "a collection that could not be resolved must be reported as it was refused")
		assert.Empty(t, c.unlocked, "and nothing may be unlocked on the strength of a collection nobody found")
	})

	t.Run("an unlock error is returned, the collection is not locked", func(t *testing.T) {
		wantErr := errors.New("dismissed")
		c := &fakeSecretServiceClient{collection: col, unlockErr: wantErr}
		b := &SecretService{Client: c, User: "alice"}

		_, err := b.List(t.Context())
		assert.ErrorIs(t, err, wantErr, "a user who dismissed the unlock dialog must be told so")
		assert.Empty(t, c.locked, "and a collection that never opened has nothing to re-lock")
	})

	t.Run("an items error is returned, the collection is still locked", func(t *testing.T) {
		wantErr := errors.New("boom")
		c := &fakeSecretServiceClient{collection: col, itemsErr: wantErr}
		b := &SecretService{Client: c, User: "alice"}

		_, err := b.List(t.Context())
		assert.ErrorIs(t, err, wantErr, "a collection whose contents could not be read must not be listed as empty")
		assert.Len(t, c.locked, 1, "and the collection this opened must not be left open behind the error")
	})

	t.Run("an attributes error is returned, the collection is still locked", func(t *testing.T) {
		wantErr := errors.New("boom")
		c := &fakeSecretServiceClient{
			collection:        col,
			itemsByCollection: map[dbus.ObjectPath][]dbus.ObjectPath{col: {item1}},
			attrsErr:          wantErr,
		}
		b := &SecretService{Client: c, User: "alice"}

		_, err := b.List(t.Context())
		assert.ErrorIs(t, err, wantErr, "an item whose attributes could not be read must not be silently dropped")
		assert.Len(t, c.locked, 1, "and the collection this opened must not be left open behind the error")
	})
}

// TestSecretServiceUnlockClientError covers Unlock's client-error branch: the
// collection resolves but the D-Bus Unlock call fails.
func TestSecretServiceUnlockClientError(t *testing.T) {
	client := &fakeSecretServiceClient{collection: "/org/collection/sshakku", unlockErr: errors.New("unlock refused")}
	b := &SecretService{Client: client, User: "u"}
	assert.Error(t, b.Unlock(t.Context()), "a collection the bus refused to unlock must not be reported as open")
}

// TestSecretServiceLockCollectionError covers Lock's resolve-failure branch:
// the collection cannot be resolved, so Lock returns before touching the bus.
func TestSecretServiceLockCollectionError(t *testing.T) {
	client := &fakeSecretServiceClient{collectionErr: errors.New("no such collection")}
	b := &SecretService{Client: client, User: "u"}
	assert.Error(t, b.Lock(t.Context()), "a collection that could not be resolved cannot be reported as locked")
	assert.Empty(t, client.locked, "and nothing may be locked on the strength of a collection nobody found")
}
