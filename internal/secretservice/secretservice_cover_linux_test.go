package secretservice

import (
	"testing"

	"github.com/godbus/dbus/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeProp is a minimal object whose Properties.Get always returns value,
// letting a test hand Client a wrong-typed property back over the real wire
// (rather than mocking the client's own accessor).
type fakeProp struct{ value any }

func (p fakeProp) Get(string, string) (dbus.Variant, *dbus.Error) {
	return dbus.MakeVariant(p.value), nil
}

// exportProp exports a fakeProp answering value on the fake service's
// connection at path, so client property reads against it decode the
// wrong-typed value from the daemon. The caller passes a literal path (rather
// than svc.nextPath, whose counter isn't synchronised against the bus reader
// goroutine) so parallel-safe subtests don't race on it.
func exportProp(t *testing.T, svc *fakeService, path dbus.ObjectPath, value any) {
	t.Helper()
	require.NoError(t, svc.conn.Export(fakeProp{value: value}, path, propsIface), "export fake prop")
}

func TestNewClientErrors(t *testing.T) {
	t.Run("connect failure surfaces", func(t *testing.T) {
		// An address pointing at a socket that doesn't exist makes
		// ConnectSessionBus fail before any Secret Service call.
		t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/nonexistent/sshakku-test-bus")
		_, err := NewClient()
		assert.Error(t, err, "an unreachable bus must be reported")
	})

	t.Run("open session failure surfaces", func(t *testing.T) {
		// Own the well-known name but export no object at the service path, so
		// the connection and name lookup succeed yet OpenSession lands on
		// nothing and fails. (Merely leaving the name unowned wouldn't do:
		// a private bus may D-Bus-activate a real secret service instead.)
		startSessionBus(t)
		serverConn, err := dbus.ConnectSessionBus()
		require.NoError(t, err, "server connect")
		t.Cleanup(func() { _ = serverConn.Close() })
		reply, err := serverConn.RequestName(busName, dbus.NameFlagDoNotQueue)
		require.NoErrorf(t, err, "request name %s", busName)
		require.Equalf(t, dbus.RequestNameReplyPrimaryOwner, reply, "request name %s", busName)

		_, err = NewClient()
		assert.Error(t, err, "an OpenSession with no object to answer it must be reported")
	})
}

func TestClientCollectionErrors(t *testing.T) {
	t.Run("ReadAlias failure surfaces", func(t *testing.T) {
		client, _ := newTestClient(t, "")
		_ = client.conn.Close()
		_, err := client.Collection("sshakku", "sshakku")
		assert.Error(t, err, "a ReadAlias that fails must be reported")
	})

	t.Run("a label-lookup failure during the alias fallback surfaces", func(t *testing.T) {
		client, svc := newTestClient(t, "")
		svc.mu.Lock()
		svc.restrictAlias = true       // force the errNotSupported fallback,
		svc.failCollectionsProp = true // then fail the by-label lookup it does
		svc.mu.Unlock()

		_, err := client.Collection("sshakku", "sshakku")
		assert.Error(t, err, "a by-label fallback lookup that fails must be reported")
	})

	t.Run("a generic CreateCollection failure surfaces", func(t *testing.T) {
		client, svc := newTestClient(t, "")
		svc.mu.Lock()
		svc.failCreateCollection = true
		svc.mu.Unlock()

		_, err := client.Collection("sshakku", "sshakku")
		assert.Error(t, err, "a CreateCollection that fails outright must be reported")

		// Reporting the error is only half of it: a failure that is not the
		// wallet refusing the alias must not be mistaken for one. Taken as a
		// refusal it would be answered by asking the wallet to make the
		// compartment a second time without an alias — a wallet that failed for
		// some other reason has not been asked twice on purpose, and the error
		// the caller finally sees is then about the wrong attempt.
		svc.mu.Lock()
		asked := svc.createCalls
		svc.mu.Unlock()
		assert.Equal(t, 1, asked, "a generic failure must not be retried without the alias")
	})

	t.Run("a non-path prompt result is an error", func(t *testing.T) {
		client, _ := newTestClient(t, "badresult")
		_, err := client.Collection("sshakku", "sshakku")
		assert.Error(t, err, "a prompt result that is not an object path must be reported")
	})
}

func TestFindCollectionByLabelErrors(t *testing.T) {
	t.Run("listing the collections can fail", func(t *testing.T) {
		client, svc := newTestClient(t, "")
		svc.mu.Lock()
		svc.failCollectionsProp = true
		svc.mu.Unlock()

		_, err := client.findCollectionByLabel("sshakku")
		assert.Error(t, err, "a Collections property read that fails must be reported")
	})

	t.Run("a wrong-typed Collections property is an error", func(t *testing.T) {
		client, svc := newTestClient(t, "")
		svc.mu.Lock()
		svc.collectionsPropSet = true
		svc.collectionsProp = "not-a-list"
		svc.mu.Unlock()

		_, err := client.findCollectionByLabel("sshakku")
		assert.Error(t, err, "a Collections property of the wrong type must be reported")
	})

	t.Run("a collection whose label read fails is skipped, not fatal", func(t *testing.T) {
		client, svc := newTestClient(t, "")
		svc.mu.Lock()
		svc.collectionsPropSet = true
		// A path the fake never exported: reading its Label errors, so the
		// loop skips it and, finding no match, returns noPrompt without error.
		svc.collectionsProp = []dbus.ObjectPath{"/org/freedesktop/secrets/collection/bogus"}
		svc.mu.Unlock()

		got, err := client.findCollectionByLabel("sshakku")
		require.NoError(t, err, "a label read that fails must be skipped, not reported")
		assert.Equal(t, noPrompt, got, "nothing matched")
	})
}

// TestClientCallErrors exercises the error path of each Client method that
// makes a single D-Bus call, by aiming it at an object path the fake never
// exported (or, for the service-level Unlock/Lock, an unknown method name):
// the daemon's owner has no such object/member, so the call fails on the wire.
func TestClientCallErrors(t *testing.T) {
	const bogusCollection = dbus.ObjectPath("/org/freedesktop/secrets/collection/nope")
	const bogusItem = dbus.ObjectPath("/org/freedesktop/secrets/item/nope")

	t.Run("unlockOrLock call failure surfaces", func(t *testing.T) {
		client, _ := newTestClient(t, "")
		err := client.unlockOrLock(serviceIface+".NoSuchMethod", []dbus.ObjectPath{bogusCollection})
		assert.Error(t, err, "an unknown service method must be reported")
	})

	t.Run("SearchItems call failure surfaces", func(t *testing.T) {
		client, _ := newTestClient(t, "")
		_, err := client.SearchItems(bogusCollection, map[string]string{"a": "b"})
		assert.Error(t, err, "searching a non-existent collection must be reported")
	})

	t.Run("GetSecret call failure surfaces", func(t *testing.T) {
		client, _ := newTestClient(t, "")
		_, err := client.GetSecret(bogusItem)
		assert.Error(t, err, "reading a non-existent item must be reported")
	})

	t.Run("CreateItem call failure surfaces", func(t *testing.T) {
		client, _ := newTestClient(t, "")
		err := client.CreateItem(bogusCollection, "x", map[string]string{"s": "v"}, "p", true)
		assert.Error(t, err, "creating an item in a non-existent collection must be reported")
	})

	t.Run("Items property read failure surfaces", func(t *testing.T) {
		client, _ := newTestClient(t, "")
		_, err := client.Items(bogusCollection)
		assert.Error(t, err, "listing a non-existent collection must be reported")
	})

	t.Run("ItemAttributes property read failure surfaces", func(t *testing.T) {
		client, _ := newTestClient(t, "")
		_, err := client.ItemAttributes(bogusItem)
		assert.Error(t, err, "reading attributes of a non-existent item must be reported")
	})

	t.Run("DeleteItem call failure surfaces", func(t *testing.T) {
		client, _ := newTestClient(t, "")
		err := client.DeleteItem(bogusItem)
		assert.Error(t, err, "deleting a non-existent item must be reported")
	})
}

func TestClientWrongTypeProperties(t *testing.T) {
	t.Run("Items rejects a wrong-typed property", func(t *testing.T) {
		client, svc := newTestClient(t, "")
		const path = dbus.ObjectPath("/org/freedesktop/secrets/badprop/items")
		exportProp(t, svc, path, "not-a-list")
		_, err := client.Items(path)
		assert.Error(t, err, "a wrong-typed Items property must be reported")
	})

	t.Run("ItemAttributes rejects a wrong-typed property", func(t *testing.T) {
		client, svc := newTestClient(t, "")
		const path = dbus.ObjectPath("/org/freedesktop/secrets/badprop/attrs")
		exportProp(t, svc, path, "not-a-map")
		_, err := client.ItemAttributes(path)
		assert.Error(t, err, "a wrong-typed Attributes property must be reported")
	})
}

func TestCompletePromptErrors(t *testing.T) {
	t.Run("watching the prompt can fail", func(t *testing.T) {
		client, _ := newTestClient(t, "")
		_ = client.conn.Close() // AddMatchSignal now fails on the closed conn
		_, err := client.completePrompt("/org/freedesktop/secrets/prompt/x")
		assert.Error(t, err, "a prompt match that cannot be registered must be reported")
	})

	t.Run("invoking the prompt can fail", func(t *testing.T) {
		client, _ := newTestClient(t, "")
		// A prompt path the fake never exported: the match registers, but the
		// Prompt() call has no object to land on.
		_, err := client.completePrompt("/org/freedesktop/secrets/prompt/unexported")
		assert.Error(t, err, "a Prompt() call with no object must be reported")
	})

	t.Run("an unexpected Completed signal is an error", func(t *testing.T) {
		client, svc := newTestClient(t, "")
		path := dbus.ObjectPath("/org/freedesktop/secrets/prompt/short")
		p := &fakePrompt{conn: svc.conn, path: path, behavior: "short"}
		require.NoError(t, svc.conn.Export(p, path, promptIface), "export prompt")
		_, err := client.completePrompt(path)
		assert.Error(t, err, "a Completed signal with the wrong shape must be reported")
	})

	t.Run("a malformed Completed signal is an error", func(t *testing.T) {
		client, svc := newTestClient(t, "")
		path := dbus.ObjectPath("/org/freedesktop/secrets/prompt/malformed")
		p := &fakePrompt{conn: svc.conn, path: path, behavior: "malformed"}
		require.NoError(t, svc.conn.Export(p, path, promptIface), "export prompt")
		_, err := client.completePrompt(path)
		assert.Error(t, err, "a malformed Completed signal must be reported")
	})
}
