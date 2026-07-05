package keys

import (
	"errors"
	"strings"
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestSecretToolLookup(t *testing.T) {
	t.Run("hit trims the trailing newline", func(t *testing.T) {
		r := newFakeRunner().on("secret-tool", stdout("hunter2\n", 0))
		b := SecretToolBackend{Runner: r, User: "alice"}
		pass, found, err := b.Lookup("SSH-Key-id_rsa")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !found || pass != "hunter2" {
			t.Fatalf("Lookup = (%q, %v), want (hunter2, true)", pass, found)
		}
		want := []string{"lookup", "service", "SSH-Key-id_rsa", "username", "alice"}
		if got := r.calls[0].Args; !equalStrings(got, want) {
			t.Fatalf("args = %v, want %v", got, want)
		}
	})

	t.Run("miss is found=false, no error", func(t *testing.T) {
		r := newFakeRunner().on("secret-tool", stdout("", 1))
		b := SecretToolBackend{Runner: r, User: "alice"}
		_, found, err := b.Lookup("SSH-Key-id_rsa")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if found {
			t.Fatal("found = true, want false for a miss")
		}
	})

	t.Run("a failure to start secret-tool is an error", func(t *testing.T) {
		wantErr := errors.New("boom")
		b := SecretToolBackend{Runner: newFakeRunner().on("secret-tool", fails(wantErr)), User: "alice"}
		if _, _, err := b.Lookup("x"); !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
	})
}

func TestSecretToolStore(t *testing.T) {
	const passphrase = "s3cr3t-pass"

	t.Run("passphrase goes on stdin, never in argv", func(t *testing.T) {
		r := newFakeRunner().on("secret-tool", stdout("", 0))
		b := SecretToolBackend{Runner: r, User: "alice"}
		if err := b.Store("SSH-Key-id_rsa", "SSH Passphrase for id_rsa", passphrase); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		call := r.calls[0]
		if call.Stdin != passphrase {
			t.Fatalf("stdin = %q, want the passphrase", call.Stdin)
		}
		for _, a := range call.Args {
			if strings.Contains(a, passphrase) {
				t.Fatalf("passphrase leaked into argv: %q", a)
			}
		}
		if call.Args[0] != "store" || call.Args[1] != "--label=SSH Passphrase for id_rsa" {
			t.Fatalf("args = %v, want store with the label", call.Args)
		}
	})

	t.Run("a non-zero exit is an error", func(t *testing.T) {
		r := newFakeRunner().on("secret-tool", func(Cmd) (Result, error) {
			return Result{Stderr: []byte("no wallet"), Code: 1}, nil
		})
		b := SecretToolBackend{Runner: r, User: "alice"}
		if err := b.Store("x", "y", passphrase); err == nil {
			t.Fatal("expected an error for a non-zero exit")
		}
	})
}

func TestSecretToolDelete(t *testing.T) {
	t.Run("clears the entry", func(t *testing.T) {
		r := newFakeRunner().on("secret-tool", stdout("", 0))
		b := SecretToolBackend{Runner: r, User: "alice"}
		if err := b.Delete("SSH-Key-id_rsa"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{"clear", "service", "SSH-Key-id_rsa", "username", "alice"}
		if got := r.calls[0].Args; !equalStrings(got, want) {
			t.Fatalf("args = %v, want %v", got, want)
		}
	})

	t.Run("a non-zero exit is an error", func(t *testing.T) {
		r := newFakeRunner().on("secret-tool", func(Cmd) (Result, error) {
			return Result{Code: 1}, nil
		})
		b := SecretToolBackend{Runner: r, User: "alice"}
		if err := b.Delete("x"); err == nil {
			t.Fatal("expected an error for a non-zero exit")
		}
	})

	t.Run("a failure to start secret-tool is an error", func(t *testing.T) {
		wantErr := errors.New("boom")
		b := SecretToolBackend{Runner: newFakeRunner().on("secret-tool", fails(wantErr)), User: "alice"}
		if err := b.Delete("x"); !errors.Is(err, wantErr) {
			t.Fatalf("error = %v, want %v", err, wantErr)
		}
	})
}

func TestSecretToolList(t *testing.T) {
	b := SecretToolBackend{Runner: newFakeRunner(), User: "alice"}
	if _, err := b.List(); !errors.Is(err, ErrListUnsupported) {
		t.Fatalf("error = %v, want %v", err, ErrListUnsupported)
	}
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

		pass, found, err := b.Lookup("SSH-Key-id_rsa")
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
		want := map[string]string{"service": "SSH-Key-id_rsa", "username": "alice"}
		if !equalAttrs(c.searchedAttrs, want) {
			t.Fatalf("searched attrs = %v, want %v", c.searchedAttrs, want)
		}
	})

	t.Run("miss is found=false, no error, still locks", func(t *testing.T) {
		c := &fakeSecretServiceClient{collection: col}
		b := &SecretServiceBackend{Client: c, User: "alice"}

		_, found, err := b.Lookup("SSH-Key-id_rsa")
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

func TestSecretServiceStore(t *testing.T) {
	const col = dbus.ObjectPath("/org/freedesktop/secrets/collection/sshakku")

	t.Run("unlocks, creates the item, and locks again", func(t *testing.T) {
		c := &fakeSecretServiceClient{collection: col}
		b := &SecretServiceBackend{Client: c, User: "alice"}

		if err := b.Store("SSH-Key-id_rsa", "SSH Passphrase for id_rsa", "hunter2"); err != nil {
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
		wantAttrs := map[string]string{"service": "SSH-Key-id_rsa", "username": "alice"}
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

		if err := b.Delete("SSH-Key-id_rsa"); err != nil {
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

		if err := b.Delete("SSH-Key-id_rsa"); err != nil {
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
				item1: {"service": "SSH-Key-id_rsa", "username": "alice"},
				item2: {"service": "SSH-Key-id_ed25519", "username": "alice"},
			},
		}
		b := &SecretServiceBackend{Client: c, User: "alice"}

		got, err := b.List()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{"SSH-Key-id_rsa", "SSH-Key-id_ed25519"}
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

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
