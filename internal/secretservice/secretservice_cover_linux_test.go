package secretservice

import (
	"testing"

	"github.com/godbus/dbus/v5"
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
	if err := svc.conn.Export(fakeProp{value: value}, path, propsIface); err != nil {
		t.Fatalf("export fake prop: %v", err)
	}
}

func TestNewClientErrors(t *testing.T) {
	t.Run("connect failure surfaces", func(t *testing.T) {
		// An address pointing at a socket that doesn't exist makes
		// ConnectSessionBus fail before any Secret Service call.
		t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/nonexistent/sshakku-test-bus")
		if _, err := NewClient(); err == nil {
			t.Fatal("NewClient returned nil error with an unreachable bus")
		}
	})

	t.Run("open session failure surfaces", func(t *testing.T) {
		// Own the well-known name but export no object at the service path, so
		// the connection and name lookup succeed yet OpenSession lands on
		// nothing and fails. (Merely leaving the name unowned wouldn't do:
		// a private bus may D-Bus-activate a real secret service instead.)
		startSessionBus(t)
		serverConn, err := dbus.ConnectSessionBus()
		if err != nil {
			t.Fatalf("server connect: %v", err)
		}
		t.Cleanup(func() { _ = serverConn.Close() })
		reply, err := serverConn.RequestName(busName, dbus.NameFlagDoNotQueue)
		if err != nil {
			t.Fatalf("request name %s: %v", busName, err)
		}
		if reply != dbus.RequestNameReplyPrimaryOwner {
			t.Fatalf("request name %s: reply = %v, want PrimaryOwner", busName, reply)
		}

		if _, err := NewClient(); err == nil {
			t.Fatal("NewClient returned nil error when OpenSession had no object to answer it")
		}
	})
}

func TestClientCollectionErrors(t *testing.T) {
	t.Run("ReadAlias failure surfaces", func(t *testing.T) {
		client, _ := newTestClient(t, "")
		_ = client.conn.Close()
		if _, err := client.Collection("sshakku", "sshakku"); err == nil {
			t.Fatal("expected an error when ReadAlias fails")
		}
	})

	t.Run("a label-lookup failure during the alias fallback surfaces", func(t *testing.T) {
		client, svc := newTestClient(t, "")
		svc.mu.Lock()
		svc.restrictAlias = true       // force the errNotSupported fallback,
		svc.failCollectionsProp = true // then fail the by-label lookup it does
		svc.mu.Unlock()

		if _, err := client.Collection("sshakku", "sshakku"); err == nil {
			t.Fatal("expected an error when the by-label fallback lookup fails")
		}
	})

	t.Run("a generic CreateCollection failure surfaces", func(t *testing.T) {
		client, svc := newTestClient(t, "")
		svc.mu.Lock()
		svc.failCreateCollection = true
		svc.mu.Unlock()

		if _, err := client.Collection("sshakku", "sshakku"); err == nil {
			t.Fatal("expected an error when CreateCollection fails outright")
		}
	})

	t.Run("a non-path prompt result is an error", func(t *testing.T) {
		client, _ := newTestClient(t, "badresult")
		if _, err := client.Collection("sshakku", "sshakku"); err == nil {
			t.Fatal("expected an error when the prompt result isn't an object path")
		}
	})
}

func TestFindCollectionByLabelErrors(t *testing.T) {
	t.Run("listing the collections can fail", func(t *testing.T) {
		client, svc := newTestClient(t, "")
		svc.mu.Lock()
		svc.failCollectionsProp = true
		svc.mu.Unlock()

		if _, err := client.findCollectionByLabel("sshakku"); err == nil {
			t.Fatal("expected an error when the Collections property read fails")
		}
	})

	t.Run("a wrong-typed Collections property is an error", func(t *testing.T) {
		client, svc := newTestClient(t, "")
		svc.mu.Lock()
		svc.collectionsPropSet = true
		svc.collectionsProp = "not-a-list"
		svc.mu.Unlock()

		if _, err := client.findCollectionByLabel("sshakku"); err == nil {
			t.Fatal("expected an error for a Collections property of the wrong type")
		}
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
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != noPrompt {
			t.Fatalf("findCollectionByLabel = %v, want noPrompt (nothing matched)", got)
		}
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
		if err := client.unlockOrLock(serviceIface+".NoSuchMethod", []dbus.ObjectPath{bogusCollection}); err == nil {
			t.Fatal("expected an error for an unknown service method")
		}
	})

	t.Run("SearchItems call failure surfaces", func(t *testing.T) {
		client, _ := newTestClient(t, "")
		if _, err := client.SearchItems(bogusCollection, map[string]string{"a": "b"}); err == nil {
			t.Fatal("expected an error searching a non-existent collection")
		}
	})

	t.Run("GetSecret call failure surfaces", func(t *testing.T) {
		client, _ := newTestClient(t, "")
		if _, err := client.GetSecret(bogusItem); err == nil {
			t.Fatal("expected an error reading a non-existent item")
		}
	})

	t.Run("CreateItem call failure surfaces", func(t *testing.T) {
		client, _ := newTestClient(t, "")
		if err := client.CreateItem(bogusCollection, "x", map[string]string{"s": "v"}, "p", true); err == nil {
			t.Fatal("expected an error creating an item in a non-existent collection")
		}
	})

	t.Run("Items property read failure surfaces", func(t *testing.T) {
		client, _ := newTestClient(t, "")
		if _, err := client.Items(bogusCollection); err == nil {
			t.Fatal("expected an error listing a non-existent collection")
		}
	})

	t.Run("ItemAttributes property read failure surfaces", func(t *testing.T) {
		client, _ := newTestClient(t, "")
		if _, err := client.ItemAttributes(bogusItem); err == nil {
			t.Fatal("expected an error reading attributes of a non-existent item")
		}
	})

	t.Run("DeleteItem call failure surfaces", func(t *testing.T) {
		client, _ := newTestClient(t, "")
		if err := client.DeleteItem(bogusItem); err == nil {
			t.Fatal("expected an error deleting a non-existent item")
		}
	})
}

func TestClientWrongTypeProperties(t *testing.T) {
	t.Run("Items rejects a wrong-typed property", func(t *testing.T) {
		client, svc := newTestClient(t, "")
		const path = dbus.ObjectPath("/org/freedesktop/secrets/badprop/items")
		exportProp(t, svc, path, "not-a-list")
		if _, err := client.Items(path); err == nil {
			t.Fatal("expected an error for a wrong-typed Items property")
		}
	})

	t.Run("ItemAttributes rejects a wrong-typed property", func(t *testing.T) {
		client, svc := newTestClient(t, "")
		const path = dbus.ObjectPath("/org/freedesktop/secrets/badprop/attrs")
		exportProp(t, svc, path, "not-a-map")
		if _, err := client.ItemAttributes(path); err == nil {
			t.Fatal("expected an error for a wrong-typed Attributes property")
		}
	})
}

func TestCompletePromptErrors(t *testing.T) {
	t.Run("watching the prompt can fail", func(t *testing.T) {
		client, _ := newTestClient(t, "")
		_ = client.conn.Close() // AddMatchSignal now fails on the closed conn
		if _, err := client.completePrompt("/org/freedesktop/secrets/prompt/x"); err == nil {
			t.Fatal("expected an error when the prompt match can't be registered")
		}
	})

	t.Run("invoking the prompt can fail", func(t *testing.T) {
		client, _ := newTestClient(t, "")
		// A prompt path the fake never exported: the match registers, but the
		// Prompt() call has no object to land on.
		if _, err := client.completePrompt("/org/freedesktop/secrets/prompt/unexported"); err == nil {
			t.Fatal("expected an error when Prompt() has no object")
		}
	})

	t.Run("an unexpected Completed signal is an error", func(t *testing.T) {
		client, svc := newTestClient(t, "")
		path := dbus.ObjectPath("/org/freedesktop/secrets/prompt/short")
		p := &fakePrompt{conn: svc.conn, path: path, behavior: "short"}
		if err := svc.conn.Export(p, path, promptIface); err != nil {
			t.Fatalf("export prompt: %v", err)
		}
		if _, err := client.completePrompt(path); err == nil {
			t.Fatal("expected an error for a Completed signal with the wrong shape")
		}
	})

	t.Run("a malformed Completed signal is an error", func(t *testing.T) {
		client, svc := newTestClient(t, "")
		path := dbus.ObjectPath("/org/freedesktop/secrets/prompt/malformed")
		p := &fakePrompt{conn: svc.conn, path: path, behavior: "malformed"}
		if err := svc.conn.Export(p, path, promptIface); err != nil {
			t.Fatalf("export prompt: %v", err)
		}
		if _, err := client.completePrompt(path); err == nil {
			t.Fatal("expected an error for a malformed Completed signal")
		}
	})
}
