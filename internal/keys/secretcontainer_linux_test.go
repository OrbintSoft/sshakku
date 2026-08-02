package keys

import (
	"testing"

	"github.com/godbus/dbus/v5"
)

// TestSecretServiceAddressesTheConfiguredContainer verifies the Linux half of
// the promise that the compartment SSHakku keeps its entries in is the one the
// configuration names (F33): the collection is addressed by that name, as both
// alias and label, and no other collection is addressed instead.
//
// Both matter. The alias is what a service that supports aliases resolves; the
// label is what one that does not falls back to matching on. A backend that
// carried the configured name in only one of them would land in the configured
// compartment on one desktop and in SSHakku's default on another, with nothing
// to tell the user which.
func TestSecretServiceAddressesTheConfiguredContainer(t *testing.T) {
	const col = dbus.ObjectPath("/org/freedesktop/secrets/collection/chosen")

	t.Run("the configured name is the alias and the label", func(t *testing.T) {
		c := &fakeSecretServiceClient{collection: col}
		b := &SecretServiceBackend{Client: c, User: "alice", Container: "my-own-compartment"}

		if _, _, err := b.Lookup("SSHakku-Key-id_rsa"); err != nil {
			t.Fatalf("Lookup: %v", err)
		}
		if len(c.collectionAsked) == 0 {
			t.Fatal("no collection was addressed at all")
		}
		for _, asked := range c.collectionAsked {
			if asked.alias != "my-own-compartment" || asked.label != "my-own-compartment" {
				t.Errorf("addressed collection (alias %q, label %q), want both to be the configured name", asked.alias, asked.label)
			}
		}
	})

	t.Run("an unset name keeps the collection SSHakku has always used", func(t *testing.T) {
		c := &fakeSecretServiceClient{collection: col}
		b := &SecretServiceBackend{Client: c, User: "alice"}

		if _, _, err := b.Lookup("SSHakku-Key-id_rsa"); err != nil {
			t.Fatalf("Lookup: %v", err)
		}
		if len(c.collectionAsked) == 0 {
			t.Fatal("no collection was addressed at all")
		}
		for _, asked := range c.collectionAsked {
			if asked.alias != secretServiceAlias || asked.label != secretServiceLabel {
				t.Errorf("addressed collection (alias %q, label %q), want the default (%q, %q)", asked.alias, asked.label, secretServiceAlias, secretServiceLabel)
			}
		}
	})

	// Every operation, not just the one a test happened to pick: a store that
	// wrote into the configured compartment while a delete emptied the default
	// one would be the same defect the entry-name setting already had.
	t.Run("every operation addresses the same one", func(t *testing.T) {
		item := dbus.ObjectPath("/item/1")
		c := &fakeSecretServiceClient{
			collection:        col,
			items:             []dbus.ObjectPath{item},
			secretsByItem:     map[dbus.ObjectPath]string{item: "hunter2"},
			itemsByCollection: map[dbus.ObjectPath][]dbus.ObjectPath{col: {item}},
			attrsByItem:       map[dbus.ObjectPath]map[string]string{item: {"service": "SSHakku-Key-id_rsa"}},
		}
		b := &SecretServiceBackend{Client: c, User: "alice", Container: "my-own-compartment"}

		if err := b.Store("SSHakku-Key-id_rsa", "label", "hunter2"); err != nil {
			t.Fatalf("Store: %v", err)
		}
		if _, err := b.List(); err != nil {
			t.Fatalf("List: %v", err)
		}
		if err := b.Delete("SSHakku-Key-id_rsa"); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		for _, asked := range c.collectionAsked {
			if asked.alias != "my-own-compartment" || asked.label != "my-own-compartment" {
				t.Errorf("addressed collection (alias %q, label %q), want the configured name for every operation", asked.alias, asked.label)
			}
		}
	})
}
