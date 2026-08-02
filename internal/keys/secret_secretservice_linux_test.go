package keys

import (
	"errors"
	"testing"

	"github.com/godbus/dbus/v5"
)

// fakeSecretServiceClient scripts SecretServiceClient for SecretServiceBackend
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

func (f *fakeSecretServiceClient) Collection(alias, label string) (dbus.ObjectPath, error) {
	f.collectionCalls++
	f.collectionAsked = append(f.collectionAsked, ssCollectionCall{alias: alias, label: label})
	if f.collectionErr != nil {
		return "", f.collectionErr
	}
	return f.collection, nil
}

func (f *fakeSecretServiceClient) Unlock(objects ...dbus.ObjectPath) error {
	f.unlocked = append(f.unlocked, objects...)
	return f.unlockErr
}

func (f *fakeSecretServiceClient) Lock(objects ...dbus.ObjectPath) error {
	f.locked = append(f.locked, objects...)
	return f.lockErr
}

func (f *fakeSecretServiceClient) SearchItems(_ dbus.ObjectPath, attrs map[string]string) ([]dbus.ObjectPath, error) {
	f.searchedAttrs = attrs
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	return f.items, nil
}

func (f *fakeSecretServiceClient) GetSecret(item dbus.ObjectPath) (string, error) {
	if f.secretErr != nil {
		return "", f.secretErr
	}
	return f.secretsByItem[item], nil
}

func (f *fakeSecretServiceClient) CreateItem(collection dbus.ObjectPath, label string, attrs map[string]string, passphrase string, replace bool) error {
	f.createdItems = append(f.createdItems, ssCreateCall{collection, label, attrs, passphrase, replace})
	return f.createErr
}

func (f *fakeSecretServiceClient) Items(collection dbus.ObjectPath) ([]dbus.ObjectPath, error) {
	if f.itemsErr != nil {
		return nil, f.itemsErr
	}
	return f.itemsByCollection[collection], nil
}

func (f *fakeSecretServiceClient) ItemAttributes(item dbus.ObjectPath) (map[string]string, error) {
	if f.attrsErr != nil {
		return nil, f.attrsErr
	}
	return f.attrsByItem[item], nil
}

func (f *fakeSecretServiceClient) DeleteItem(item dbus.ObjectPath) error {
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
		b := &SecretServiceBackend{Client: c, User: "alice"}

		pass, found, err := b.Lookup(defaultServicePrefix + "-id_rsa")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !found || pass != "hunter2" {
			t.Fatalf("Lookup = (%q, %v), want (hunter2, true)", pass, found)
		}
		if len(c.unlocked) != 1 || c.unlocked[0] != col {
			t.Fatalf("unlocked = %v, want [%v]", c.unlocked, col)
		}
		if len(c.locked) != 1 || c.locked[0] != col {
			t.Fatalf("locked = %v, want [%v]", c.locked, col)
		}
		want := map[string]string{"service": defaultServicePrefix + "-id_rsa", "username": "alice"}
		if !equalAttrs(c.searchedAttrs, want) {
			t.Fatalf("searched attrs = %v, want %v", c.searchedAttrs, want)
		}
	})

	t.Run("miss is found=false, no error, still locks", func(t *testing.T) {
		c := &fakeSecretServiceClient{collection: col}
		b := &SecretServiceBackend{Client: c, User: "alice"}

		_, found, err := b.Lookup(defaultServicePrefix + "-id_rsa")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if found {
			t.Fatal("found = true, want false for a miss")
		}
		if len(c.locked) != 1 {
			t.Fatalf("locked = %v, want the collection locked even on a miss", c.locked)
		}
	})

	t.Run("the collection is resolved once and cached", func(t *testing.T) {
		c := &fakeSecretServiceClient{collection: col}
		b := &SecretServiceBackend{Client: c, User: "alice"}

		if _, _, err := b.Lookup("a"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, _, err := b.Lookup("b"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if c.collectionCalls != 1 {
			t.Fatalf("Collection called %d times, want 1", c.collectionCalls)
		}
	})

	t.Run("a collection error is returned, nothing is unlocked", func(t *testing.T) {
		wantErr := errors.New("boom")
		c := &fakeSecretServiceClient{collectionErr: wantErr}
		b := &SecretServiceBackend{Client: c, User: "alice"}

		if _, _, err := b.Lookup("x"); !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
		if len(c.unlocked) != 0 {
			t.Fatalf("unlocked = %v, want none", c.unlocked)
		}
	})

	t.Run("an unlock error is returned, the collection is not locked", func(t *testing.T) {
		wantErr := errors.New("dismissed")
		c := &fakeSecretServiceClient{collection: col, unlockErr: wantErr}
		b := &SecretServiceBackend{Client: c, User: "alice"}

		if _, _, err := b.Lookup("x"); !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
		if len(c.locked) != 0 {
			t.Fatalf("locked = %v, want none: an unlock failure has nothing to re-lock", c.locked)
		}
	})

	t.Run("a search error is returned, the collection is still locked", func(t *testing.T) {
		wantErr := errors.New("boom")
		c := &fakeSecretServiceClient{collection: col, searchErr: wantErr}
		b := &SecretServiceBackend{Client: c, User: "alice"}

		if _, _, err := b.Lookup("x"); !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
		if len(c.locked) != 1 {
			t.Fatalf("locked = %v, want the collection locked despite the search error", c.locked)
		}
	})

	t.Run("a get-secret error is returned, the collection is still locked", func(t *testing.T) {
		wantErr := errors.New("boom")
		c := &fakeSecretServiceClient{collection: col, items: []dbus.ObjectPath{item}, secretErr: wantErr}
		b := &SecretServiceBackend{Client: c, User: "alice"}

		_, found, err := b.Lookup("x")
		if !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
		if found {
			t.Fatal("found = true, want false on a get-secret error")
		}
		if len(c.locked) != 1 {
			t.Fatalf("locked = %v, want the collection locked despite the get-secret error", c.locked)
		}
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
		b := &SecretServiceBackend{Client: c, User: "alice"}

		if err := b.Unlock(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, _, err := b.Lookup("a"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := b.Store("b", "label", "hunter2"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(c.unlocked) != 1 {
			t.Fatalf("unlocked = %v, want exactly one explicit unlock shared across calls", c.unlocked)
		}
		if len(c.locked) != 0 {
			t.Fatalf("locked = %v, want none until Lock is called", c.locked)
		}

		if err := b.Lock(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(c.locked) != 1 {
			t.Fatalf("locked = %v, want exactly one lock after Lock()", c.locked)
		}

		// held is cleared after Lock: a further call unlocks and locks on its own again.
		if _, _, err := b.Lookup("c"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(c.unlocked) != 2 || len(c.locked) != 2 {
			t.Fatalf("unlocked=%v locked=%v, want a fresh unlock/lock bracket after Lock()", c.unlocked, c.locked)
		}
	})

	t.Run("an Unlock collection error leaves held false", func(t *testing.T) {
		wantErr := errors.New("boom")
		c := &fakeSecretServiceClient{collectionErr: wantErr}
		b := &SecretServiceBackend{Client: c, User: "alice"}

		if err := b.Unlock(); !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
		if b.held {
			t.Fatal("held = true after a failed Unlock, want false")
		}
	})
}

func TestSecretServiceStore(t *testing.T) {
	const col = dbus.ObjectPath("/org/freedesktop/secrets/collection/sshakku")

	t.Run("unlocks, creates the item, and locks again", func(t *testing.T) {
		c := &fakeSecretServiceClient{collection: col}
		b := &SecretServiceBackend{Client: c, User: "alice"}

		if err := b.Store(defaultServicePrefix+"-id_rsa", "SSH Passphrase for id_rsa", "hunter2"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(c.unlocked) != 1 || c.unlocked[0] != col {
			t.Fatalf("unlocked = %v, want [%v]", c.unlocked, col)
		}
		if len(c.locked) != 1 || c.locked[0] != col {
			t.Fatalf("locked = %v, want [%v]", c.locked, col)
		}
		if len(c.createdItems) != 1 {
			t.Fatalf("createdItems = %v, want exactly one", c.createdItems)
		}
		got := c.createdItems[0]
		wantAttrs := map[string]string{"service": defaultServicePrefix + "-id_rsa", "username": "alice"}
		if got.collection != col || got.label != "SSH Passphrase for id_rsa" || got.passphrase != "hunter2" || !got.replace || !equalAttrs(got.attrs, wantAttrs) {
			t.Fatalf("CreateItem call = %+v, want collection=%v label=%q passphrase=hunter2 replace=true attrs=%v", got, col, "SSH Passphrase for id_rsa", wantAttrs)
		}
	})

	t.Run("a collection error is returned, nothing is unlocked", func(t *testing.T) {
		wantErr := errors.New("boom")
		c := &fakeSecretServiceClient{collectionErr: wantErr}
		b := &SecretServiceBackend{Client: c, User: "alice"}

		if err := b.Store("x", "y", "z"); !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
		if len(c.unlocked) != 0 {
			t.Fatalf("unlocked = %v, want none", c.unlocked)
		}
	})

	t.Run("an unlock error is returned, the collection is not locked", func(t *testing.T) {
		wantErr := errors.New("dismissed")
		c := &fakeSecretServiceClient{collection: col, unlockErr: wantErr}
		b := &SecretServiceBackend{Client: c, User: "alice"}

		if err := b.Store("x", "y", "z"); !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
		if len(c.locked) != 0 {
			t.Fatalf("locked = %v, want none: an unlock failure has nothing to re-lock", c.locked)
		}
	})

	t.Run("a create-item error is returned, the collection is still locked", func(t *testing.T) {
		wantErr := errors.New("boom")
		c := &fakeSecretServiceClient{collection: col, createErr: wantErr}
		b := &SecretServiceBackend{Client: c, User: "alice"}

		if err := b.Store("x", "y", "z"); !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
		if len(c.locked) != 1 {
			t.Fatalf("locked = %v, want the collection locked despite the create-item error", c.locked)
		}
	})
}

func TestSecretServiceDelete(t *testing.T) {
	const col = dbus.ObjectPath("/org/freedesktop/secrets/collection/sshakku")
	const item = dbus.ObjectPath("/org/freedesktop/secrets/collection/sshakku/1")

	t.Run("hit unlocks, deletes the matching item, and locks again", func(t *testing.T) {
		c := &fakeSecretServiceClient{collection: col, items: []dbus.ObjectPath{item}}
		b := &SecretServiceBackend{Client: c, User: "alice"}

		if err := b.Delete(defaultServicePrefix + "-id_rsa"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(c.unlocked) != 1 || c.unlocked[0] != col {
			t.Fatalf("unlocked = %v, want [%v]", c.unlocked, col)
		}
		if len(c.locked) != 1 || c.locked[0] != col {
			t.Fatalf("locked = %v, want [%v]", c.locked, col)
		}
		if len(c.deletedItems) != 1 || c.deletedItems[0] != item {
			t.Fatalf("deletedItems = %v, want [%v]", c.deletedItems, item)
		}
	})

	t.Run("a miss is success, no error, still locks, nothing deleted", func(t *testing.T) {
		c := &fakeSecretServiceClient{collection: col}
		b := &SecretServiceBackend{Client: c, User: "alice"}

		if err := b.Delete(defaultServicePrefix + "-id_rsa"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(c.locked) != 1 {
			t.Fatalf("locked = %v, want the collection locked even on a miss", c.locked)
		}
		if len(c.deletedItems) != 0 {
			t.Fatalf("deletedItems = %v, want none", c.deletedItems)
		}
	})

	t.Run("a collection error is returned, nothing is unlocked", func(t *testing.T) {
		wantErr := errors.New("boom")
		c := &fakeSecretServiceClient{collectionErr: wantErr}
		b := &SecretServiceBackend{Client: c, User: "alice"}

		if err := b.Delete("x"); !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
		if len(c.unlocked) != 0 {
			t.Fatalf("unlocked = %v, want none", c.unlocked)
		}
	})

	t.Run("an unlock error is returned, the collection is not locked", func(t *testing.T) {
		wantErr := errors.New("dismissed")
		c := &fakeSecretServiceClient{collection: col, unlockErr: wantErr}
		b := &SecretServiceBackend{Client: c, User: "alice"}

		if err := b.Delete("x"); !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
		if len(c.locked) != 0 {
			t.Fatalf("locked = %v, want none: an unlock failure has nothing to re-lock", c.locked)
		}
	})

	t.Run("a search error is returned, the collection is still locked", func(t *testing.T) {
		wantErr := errors.New("boom")
		c := &fakeSecretServiceClient{collection: col, searchErr: wantErr}
		b := &SecretServiceBackend{Client: c, User: "alice"}

		if err := b.Delete("x"); !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
		if len(c.locked) != 1 {
			t.Fatalf("locked = %v, want the collection locked despite the search error", c.locked)
		}
	})

	t.Run("a delete-item error is returned, the collection is still locked", func(t *testing.T) {
		wantErr := errors.New("boom")
		c := &fakeSecretServiceClient{collection: col, items: []dbus.ObjectPath{item}, deleteItemErr: wantErr}
		b := &SecretServiceBackend{Client: c, User: "alice"}

		if err := b.Delete("x"); !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
		if len(c.locked) != 1 {
			t.Fatalf("locked = %v, want the collection locked despite the delete-item error", c.locked)
		}
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
		b := &SecretServiceBackend{Client: c, User: "alice"}

		got, err := b.List()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{defaultServicePrefix + "-id_rsa", defaultServicePrefix + "-id_ed25519"}
		if !equalStrings(got, want) {
			t.Fatalf("List = %v, want %v", got, want)
		}
		if len(c.unlocked) != 1 || c.unlocked[0] != col {
			t.Fatalf("unlocked = %v, want [%v]", c.unlocked, col)
		}
		if len(c.locked) != 1 || c.locked[0] != col {
			t.Fatalf("locked = %v, want [%v]", c.locked, col)
		}
	})

	t.Run("an empty collection returns none, no error", func(t *testing.T) {
		c := &fakeSecretServiceClient{collection: col}
		b := &SecretServiceBackend{Client: c, User: "alice"}

		got, err := b.List()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("List = %v, want none", got)
		}
	})

	t.Run("a collection error is returned, nothing is unlocked", func(t *testing.T) {
		wantErr := errors.New("boom")
		c := &fakeSecretServiceClient{collectionErr: wantErr}
		b := &SecretServiceBackend{Client: c, User: "alice"}

		if _, err := b.List(); !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
		if len(c.unlocked) != 0 {
			t.Fatalf("unlocked = %v, want none", c.unlocked)
		}
	})

	t.Run("an unlock error is returned, the collection is not locked", func(t *testing.T) {
		wantErr := errors.New("dismissed")
		c := &fakeSecretServiceClient{collection: col, unlockErr: wantErr}
		b := &SecretServiceBackend{Client: c, User: "alice"}

		if _, err := b.List(); !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
		if len(c.locked) != 0 {
			t.Fatalf("locked = %v, want none: an unlock failure has nothing to re-lock", c.locked)
		}
	})

	t.Run("an items error is returned, the collection is still locked", func(t *testing.T) {
		wantErr := errors.New("boom")
		c := &fakeSecretServiceClient{collection: col, itemsErr: wantErr}
		b := &SecretServiceBackend{Client: c, User: "alice"}

		if _, err := b.List(); !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
		if len(c.locked) != 1 {
			t.Fatalf("locked = %v, want the collection locked despite the items error", c.locked)
		}
	})

	t.Run("an attributes error is returned, the collection is still locked", func(t *testing.T) {
		wantErr := errors.New("boom")
		c := &fakeSecretServiceClient{
			collection:        col,
			itemsByCollection: map[dbus.ObjectPath][]dbus.ObjectPath{col: {item1}},
			attrsErr:          wantErr,
		}
		b := &SecretServiceBackend{Client: c, User: "alice"}

		if _, err := b.List(); !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
		if len(c.locked) != 1 {
			t.Fatalf("locked = %v, want the collection locked despite the attributes error", c.locked)
		}
	})
}

// TestSecretServiceUnlockClientError covers Unlock's client-error branch: the
// collection resolves but the D-Bus Unlock call fails.
func TestSecretServiceUnlockClientError(t *testing.T) {
	client := &fakeSecretServiceClient{collection: "/org/collection/sshakku", unlockErr: errors.New("unlock refused")}
	b := &SecretServiceBackend{Client: client, User: "u"}
	if err := b.Unlock(); err == nil {
		t.Fatal("Unlock returned nil, want the client's unlock error")
	}
}

// TestSecretServiceLockCollectionError covers Lock's resolve-failure branch:
// the collection cannot be resolved, so Lock returns before touching the bus.
func TestSecretServiceLockCollectionError(t *testing.T) {
	client := &fakeSecretServiceClient{collectionErr: errors.New("no such collection")}
	b := &SecretServiceBackend{Client: client, User: "u"}
	if err := b.Lock(); err == nil {
		t.Fatal("Lock returned nil, want the collection-resolve error")
	}
	if len(client.locked) != 0 {
		t.Fatalf("Lock must not call the bus when the collection cannot resolve, got %v", client.locked)
	}
}

func equalAttrs(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
