package secretservice

import (
	"context"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestClient starts a private session bus, exports a fakeService on it
// with the given prompt behavior, and returns a Client connected to it
// alongside the fake service for assertions/seeding.
func newTestClient(t *testing.T, behavior string) (*Client, *fakeService) {
	t.Helper()
	startSessionBus(t)

	serverConn, err := dbus.ConnectSessionBus()
	require.NoError(t, err, "server connect")
	t.Cleanup(func() { _ = serverConn.Close() })
	svc := startFakeSecretService(t, serverConn, behavior)

	client, err := NewClient(t.Context())
	require.NoError(t, err, "NewClient")
	t.Cleanup(func() { _ = client.Close(t.Context()) })

	return client, svc
}

// oneItemCollection opens a client against a fake service behaving as
// `behavior` says, and leaves exactly one item in the sshakku collection for
// the caller to act on. The item's name, attributes and secret are stand-ins:
// what the tests built on it are about is what happens to the item, not what
// is in it.
func oneItemCollection(t *testing.T, behavior string) (client *Client, svc *fakeService, col, item dbus.ObjectPath) {
	t.Helper()
	client, svc = newTestClient(t, behavior)

	var err error
	col, err = client.Collection(t.Context(), "sshakku", "sshakku")
	require.NoError(t, err, "Collection")
	require.NoError(t, client.CreateItem(t.Context(), col, "x", map[string]string{"service": "s"}, "p", true), "CreateItem")

	items, err := client.Items(t.Context(), col)
	require.NoError(t, err, "Items")
	require.Len(t, items, 1, "exactly one item")

	return client, svc, col, items[0]
}

func TestClientCollection(t *testing.T) {
	t.Run("an existing alias is returned without creating", func(t *testing.T) {
		client, svc := newTestClient(t, "")
		const existing = dbus.ObjectPath("/org/freedesktop/secrets/collection/existing")
		svc.mu.Lock()
		svc.aliases["sshakku"] = existing
		svc.mu.Unlock()

		got, err := client.Collection(t.Context(), "sshakku", "sshakku")
		require.NoError(t, err, "Collection")
		assert.Equal(t, existing, got, "the aliased collection must be returned as it is")
	})

	t.Run("creates the collection immediately when no prompt is needed", func(t *testing.T) {
		client, _ := newTestClient(t, "")
		got, err := client.Collection(t.Context(), "sshakku", "sshakku")
		require.NoError(t, err, "Collection")
		assert.NotEmpty(t, got, "a real object path")
		assert.NotEqual(t, noPrompt, got, "a real object path, not the no-prompt sentinel")
	})

	t.Run("creates the collection via a completed prompt", func(t *testing.T) {
		client, _ := newTestClient(t, "ok")
		got, err := client.Collection(t.Context(), "sshakku", "sshakku")
		require.NoError(t, err, "Collection")
		assert.NotEmpty(t, got, "a real object path")
		assert.NotEqual(t, noPrompt, got, "a real object path, not the no-prompt sentinel")
	})

	t.Run("a dismissed prompt is an error", func(t *testing.T) {
		client, _ := newTestClient(t, "dismiss")
		_, err := client.Collection(t.Context(), "sshakku", "sshakku")
		assert.Error(t, err, "a dismissed prompt must be reported")
	})

	// GNOME Keyring rejects CreateCollection for any alias other than ""
	// or "default" (errNotSupported) — unlike KDE's ksecretd, which accepts
	// an arbitrary alias. These reproduce that against the real wire
	// protocol via fakeService.restrictAlias.
	t.Run("falls back to an unaliased create when the alias is not supported", func(t *testing.T) {
		client, svc := newTestClient(t, "")
		svc.mu.Lock()
		svc.restrictAlias = true
		svc.mu.Unlock()

		got, err := client.Collection(t.Context(), "sshakku", "sshakku")
		require.NoError(t, err, "Collection")
		assert.NotEmpty(t, got, "a real object path")
		assert.NotEqual(t, noPrompt, got, "a real object path, not the no-prompt sentinel")
		svc.mu.Lock()
		_, aliased := svc.aliases["sshakku"]
		asked := svc.createCalls
		svc.mu.Unlock()
		assert.False(t, aliased, "no alias may be set when the wallet refuses one")
		assert.Equal(t, 2, asked, "the refused alias must be answered by asking again without one")
	})

	t.Run("falls back to a completed prompt when the alias is not supported", func(t *testing.T) {
		client, svc := newTestClient(t, "ok")
		svc.mu.Lock()
		svc.restrictAlias = true
		svc.mu.Unlock()

		got, err := client.Collection(t.Context(), "sshakku", "sshakku")
		require.NoError(t, err, "Collection")
		assert.NotEmpty(t, got, "a real object path")
		assert.NotEqual(t, noPrompt, got, "a real object path, not the no-prompt sentinel")
	})

	t.Run("finds an existing collection by label when the alias is not supported", func(t *testing.T) {
		client, svc := newTestClient(t, "")
		svc.mu.Lock()
		svc.restrictAlias = true
		svc.mu.Unlock()

		first, err := client.Collection(t.Context(), "sshakku", "sshakku")
		require.NoError(t, err, "first Collection call")

		second, err := client.Collection(t.Context(), "sshakku", "sshakku")
		require.NoError(t, err, "second Collection call")
		assert.Equal(t, first, second, "the second call must find the first by label, not recreate it")

		svc.mu.Lock()
		n := len(svc.collections)
		svc.mu.Unlock()
		assert.Equal(t, 1, n, "only one collection may have been created")
	})
}

func TestClientUnlockLock(t *testing.T) {
	const col = dbus.ObjectPath("/org/freedesktop/secrets/collection/x")

	t.Run("completes immediately when no prompt is needed", func(t *testing.T) {
		client, _ := newTestClient(t, "")
		assert.NoError(t, client.Unlock(t.Context(), col), "Unlock")
		assert.NoError(t, client.Lock(t.Context(), col), "Lock")
	})

	t.Run("completes via a completed prompt", func(t *testing.T) {
		client, _ := newTestClient(t, "ok")
		assert.NoError(t, client.Unlock(t.Context(), col), "Unlock")
	})

	t.Run("a dismissed prompt is an error", func(t *testing.T) {
		client, _ := newTestClient(t, "dismiss")
		assert.Error(t, client.Unlock(t.Context(), col), "a dismissed prompt must be reported")
	})

	t.Run("a hung prompt times out and is dismissed", func(t *testing.T) {
		orig := defaultPromptTimeout
		defaultPromptTimeout = 200 * time.Millisecond
		defer func() { defaultPromptTimeout = orig }()

		client, svc := newTestClient(t, "hang")
		start := time.Now()
		assert.Error(t, client.Unlock(t.Context(), col), "a prompt that never completes must be reported")
		assert.Less(t, time.Since(start), 2*time.Second, "the shortened prompt budget must be the one in force")

		svc.mu.Lock()
		prompt := svc.lastPrompt
		svc.mu.Unlock()
		require.NotNil(t, prompt, "a prompt must have been created")
		prompt.mu.Lock()
		dismissed := prompt.dismissedCalls
		prompt.mu.Unlock()
		assert.NotZero(t, dismissed, "the timed-out prompt must be dismissed")
	})

	t.Run("a cancelled caller stops waiting on the prompt", func(t *testing.T) {
		client, svc := newTestClient(t, "hang")
		// Long enough that a budget still being what ends this wait is a
		// failure nobody can mistake for a fast cancellation.
		client.PromptTimeout = 30 * time.Second

		ctx, cancel := context.WithCancel(t.Context())
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()

		start := time.Now()
		assert.Error(t, client.Unlock(ctx, col), "a caller that gave up must not be told the wallet was unlocked")
		assert.Less(t, time.Since(start), 5*time.Second,
			"the wait must end with the caller, not with the prompt budget")

		svc.mu.Lock()
		prompt := svc.lastPrompt
		svc.mu.Unlock()
		require.NotNil(t, prompt, "a prompt must have been created")
		prompt.mu.Lock()
		dismissed := prompt.dismissedCalls
		prompt.mu.Unlock()
		assert.NotZero(t, dismissed,
			"and the dialog must be taken off the user's screen, since nothing is waiting for their answer any more")
	})
}

func TestClientSearchCreateGetSecret(t *testing.T) {
	client, _ := newTestClient(t, "")
	col, err := client.Collection(t.Context(), "sshakku", "sshakku")
	require.NoError(t, err, "Collection")
	attrs := map[string]string{"service": "test-service-id_rsa", "username": "alice"}

	t.Run("a search with no match is empty, not an error", func(t *testing.T) {
		items, err := client.SearchItems(t.Context(), col, attrs)
		require.NoError(t, err, "SearchItems")
		assert.Empty(t, items, "nothing has been stored yet")
	})

	require.NoError(t, client.CreateItem(t.Context(), col, "SSH Passphrase for id_rsa", attrs, "hunter2", true), "CreateItem")

	t.Run("the created item is found and its secret reads back", func(t *testing.T) {
		items, err := client.SearchItems(t.Context(), col, attrs)
		require.NoError(t, err, "SearchItems")
		require.Len(t, items, 1, "exactly one match")
		pass, err := client.GetSecret(t.Context(), items[0])
		require.NoError(t, err, "GetSecret")
		assert.Equal(t, "hunter2", pass, "the secret that was stored")
	})

	t.Run("replace=true overwrites in place instead of duplicating", func(t *testing.T) {
		require.NoError(t, client.CreateItem(t.Context(), col, "renamed", attrs, "newpass", true), "CreateItem (replace)")
		items, err := client.SearchItems(t.Context(), col, attrs)
		require.NoError(t, err, "SearchItems")
		require.Len(t, items, 1, "still exactly one item after a replace")
		pass, err := client.GetSecret(t.Context(), items[0])
		require.NoError(t, err, "GetSecret")
		assert.Equal(t, "newpass", pass, "the replacing secret")
	})
}

func TestClientItemsAttributesDelete(t *testing.T) {
	t.Run("Items and ItemAttributes reflect what was created", func(t *testing.T) {
		client, _ := newTestClient(t, "")
		col, err := client.Collection(t.Context(), "sshakku", "sshakku")
		require.NoError(t, err, "Collection")

		items, err := client.Items(t.Context(), col)
		require.NoError(t, err, "Items on an empty collection")
		assert.Empty(t, items, "an empty collection holds nothing")

		attrs := map[string]string{"service": "test-service-id_rsa", "username": "alice"}
		require.NoError(t, client.CreateItem(t.Context(), col, "SSH Passphrase for id_rsa", attrs, "hunter2", true), "CreateItem")

		items, err = client.Items(t.Context(), col)
		require.NoError(t, err, "Items")
		require.Len(t, items, 1, "exactly one item")

		got, err := client.ItemAttributes(t.Context(), items[0])
		require.NoError(t, err, "ItemAttributes")
		assert.Equal(t, attrs["service"], got["service"], "the service attribute")
		assert.Equal(t, attrs["username"], got["username"], "the username attribute")
	})

	t.Run("DeleteItem removes the item immediately when no prompt is needed", func(t *testing.T) {
		client, _, col, item := oneItemCollection(t, "")

		require.NoError(t, client.DeleteItem(t.Context(), item), "DeleteItem")
		items, err := client.Items(t.Context(), col)
		require.NoError(t, err, "Items after DeleteItem")
		assert.Empty(t, items, "the item must be gone")
	})

	t.Run("DeleteItem completes via a completed prompt", func(t *testing.T) {
		client, _, col, item := oneItemCollection(t, "ok")

		require.NoError(t, client.DeleteItem(t.Context(), item), "DeleteItem")
		items, err := client.Items(t.Context(), col)
		require.NoError(t, err, "Items after DeleteItem")
		assert.Empty(t, items, "the item must be gone")
	})

	t.Run("a dismissed prompt leaves the item in place and is an error", func(t *testing.T) {
		client, svc, col, item := oneItemCollection(t, "")

		svc.mu.Lock()
		svc.behavior = "dismiss"
		svc.mu.Unlock()

		assert.Error(t, client.DeleteItem(t.Context(), item), "a dismissed prompt must be reported")
		items, err := client.Items(t.Context(), col)
		require.NoError(t, err, "Items after a dismissed DeleteItem")
		assert.Len(t, items, 1, "the item must still be there")
	})
}
