package secretservice

import (
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

// newTestClient starts a private session bus, exports a fakeService on it
// with the given prompt behavior, and returns a Client connected to it
// alongside the fake service for assertions/seeding.
func newTestClient(t *testing.T, behavior string) (*Client, *fakeService) {
	t.Helper()
	startSessionBus(t)

	serverConn, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = serverConn.Close() })
	svc := startFakeSecretService(t, serverConn, behavior)

	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	return client, svc
}

func TestClientCollection(t *testing.T) {
	t.Run("an existing alias is returned without creating", func(t *testing.T) {
		client, svc := newTestClient(t, "")
		const existing = dbus.ObjectPath("/org/freedesktop/secrets/collection/existing")
		svc.mu.Lock()
		svc.aliases["sshakku"] = existing
		svc.mu.Unlock()

		got, err := client.Collection("sshakku", "sshakku")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != existing {
			t.Fatalf("Collection = %v, want %v", got, existing)
		}
	})

	t.Run("creates the collection immediately when no prompt is needed", func(t *testing.T) {
		client, _ := newTestClient(t, "")
		got, err := client.Collection("sshakku", "sshakku")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == "" || got == noPrompt {
			t.Fatalf("Collection = %v, want a real object path", got)
		}
	})

	t.Run("creates the collection via a completed prompt", func(t *testing.T) {
		client, _ := newTestClient(t, "ok")
		got, err := client.Collection("sshakku", "sshakku")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == "" || got == noPrompt {
			t.Fatalf("Collection = %v, want a real object path", got)
		}
	})

	t.Run("a dismissed prompt is an error", func(t *testing.T) {
		client, _ := newTestClient(t, "dismiss")
		if _, err := client.Collection("sshakku", "sshakku"); err == nil {
			t.Fatal("expected an error for a dismissed prompt")
		}
	})
}

func TestClientUnlockLock(t *testing.T) {
	const col = dbus.ObjectPath("/org/freedesktop/secrets/collection/x")

	t.Run("completes immediately when no prompt is needed", func(t *testing.T) {
		client, _ := newTestClient(t, "")
		if err := client.Unlock(col); err != nil {
			t.Fatalf("Unlock: %v", err)
		}
		if err := client.Lock(col); err != nil {
			t.Fatalf("Lock: %v", err)
		}
	})

	t.Run("completes via a completed prompt", func(t *testing.T) {
		client, _ := newTestClient(t, "ok")
		if err := client.Unlock(col); err != nil {
			t.Fatalf("Unlock: %v", err)
		}
	})

	t.Run("a dismissed prompt is an error", func(t *testing.T) {
		client, _ := newTestClient(t, "dismiss")
		if err := client.Unlock(col); err == nil {
			t.Fatal("expected an error for a dismissed prompt")
		}
	})

	t.Run("a hung prompt times out and is dismissed", func(t *testing.T) {
		orig := promptTimeout
		promptTimeout = 200 * time.Millisecond
		defer func() { promptTimeout = orig }()

		client, svc := newTestClient(t, "hang")
		start := time.Now()
		if err := client.Unlock(col); err == nil {
			t.Fatal("expected a timeout error")
		}
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Fatalf("Unlock took %v, want close to the shortened promptTimeout", elapsed)
		}

		svc.mu.Lock()
		prompt := svc.lastPrompt
		svc.mu.Unlock()
		if prompt == nil {
			t.Fatal("no prompt was created")
		}
		prompt.mu.Lock()
		dismissed := prompt.dismissedCalls
		prompt.mu.Unlock()
		if dismissed == 0 {
			t.Fatal("expected the timed-out prompt to be dismissed")
		}
	})
}

func TestClientSearchCreateGetSecret(t *testing.T) {
	client, _ := newTestClient(t, "")
	col, err := client.Collection("sshakku", "sshakku")
	if err != nil {
		t.Fatalf("Collection: %v", err)
	}
	attrs := map[string]string{"service": "SSH-Key-id_rsa", "username": "alice"}

	t.Run("a search with no match is empty, not an error", func(t *testing.T) {
		items, err := client.SearchItems(col, attrs)
		if err != nil {
			t.Fatalf("SearchItems: %v", err)
		}
		if len(items) != 0 {
			t.Fatalf("SearchItems = %v, want none", items)
		}
	})

	if err := client.CreateItem(col, "SSH Passphrase for id_rsa", attrs, "hunter2", true); err != nil {
		t.Fatalf("CreateItem: %v", err)
	}

	t.Run("the created item is found and its secret reads back", func(t *testing.T) {
		items, err := client.SearchItems(col, attrs)
		if err != nil {
			t.Fatalf("SearchItems: %v", err)
		}
		if len(items) != 1 {
			t.Fatalf("SearchItems = %v, want exactly one match", items)
		}
		pass, err := client.GetSecret(items[0])
		if err != nil {
			t.Fatalf("GetSecret: %v", err)
		}
		if pass != "hunter2" {
			t.Fatalf("GetSecret = %q, want hunter2", pass)
		}
	})

	t.Run("replace=true overwrites in place instead of duplicating", func(t *testing.T) {
		if err := client.CreateItem(col, "renamed", attrs, "newpass", true); err != nil {
			t.Fatalf("CreateItem (replace): %v", err)
		}
		items, err := client.SearchItems(col, attrs)
		if err != nil {
			t.Fatalf("SearchItems: %v", err)
		}
		if len(items) != 1 {
			t.Fatalf("SearchItems after replace = %v, want still exactly one item", items)
		}
		pass, err := client.GetSecret(items[0])
		if err != nil {
			t.Fatalf("GetSecret: %v", err)
		}
		if pass != "newpass" {
			t.Fatalf("GetSecret after replace = %q, want newpass", pass)
		}
	})
}
