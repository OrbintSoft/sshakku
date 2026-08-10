package keys

import (
	"testing"

	"github.com/godbus/dbus/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

		_, _, err := b.Lookup("SSHakku-Key-id_rsa")
		require.NoError(t, err, "a lookup in the configured compartment must succeed")
		require.NotEmpty(t, c.collectionAsked, "some compartment must have been addressed")
		for _, asked := range c.collectionAsked {
			assert.Equal(t, "my-own-compartment", asked.alias,
				"a service that supports aliases resolves this one, so it must carry the configured name")
			assert.Equal(t, "my-own-compartment", asked.label,
				"and a service that does not falls back to matching the label, so it must carry it too")
		}
	})

	t.Run("an unset name keeps the collection SSHakku has always used", func(t *testing.T) {
		c := &fakeSecretServiceClient{collection: col}
		b := &SecretServiceBackend{Client: c, User: "alice"}

		_, _, err := b.Lookup("SSHakku-Key-id_rsa")
		require.NoError(t, err, "a lookup in SSHakku's own compartment must succeed")
		require.NotEmpty(t, c.collectionAsked, "some compartment must have been addressed")
		for _, asked := range c.collectionAsked {
			assert.Equal(t, secretServiceAlias, asked.alias,
				"a user who configured nothing must keep the compartment their entries are already in")
			assert.Equal(t, secretServiceLabel, asked.label, "addressed by its label as well")
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

		require.NoError(t, b.Store("SSHakku-Key-id_rsa", "label", "hunter2"), "a store must succeed")
		_, err := b.List()
		require.NoError(t, err, "a listing must succeed")
		require.NoError(t, b.Delete("SSHakku-Key-id_rsa"), "a delete must succeed")
		require.NotEmpty(t, c.collectionAsked, "some compartment must have been addressed")
		for _, asked := range c.collectionAsked {
			assert.Equal(t, "my-own-compartment", asked.alias,
				"a store that wrote into the configured compartment while a delete emptied the default one "+
					"would leave the user's passphrases in one place and SSHakku looking in another")
			assert.Equal(t, "my-own-compartment", asked.label, "addressed by its label as well")
		}
	})
}
