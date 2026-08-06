//go:build linux

package secretservice

import (
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

// lookTestTimeout is short on purpose: a look that has to be waited for is the
// case these tests are here to catch.
const lookTestTimeout = 2 * time.Second

// startFakeOnBus puts a fake Secret Service on a private session bus and leaves
// DBUS_SESSION_BUS_ADDRESS pointing at it, which is where LookForCollection
// looks. Unlike newTestClient it hands back no Client: the look opens its own
// connection, which is part of what is being tested.
func startFakeOnBus(t *testing.T) *fakeService {
	t.Helper()
	startSessionBus(t)

	serverConn, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = serverConn.Close() })
	return startFakeSecretService(t, serverConn, "")
}

func TestLookForCollection(t *testing.T) {
	t.Run("a wallet answering, with the compartment already there", func(t *testing.T) {
		svc := startFakeOnBus(t)
		const existing = dbus.ObjectPath("/org/freedesktop/secrets/collection/existing")
		svc.mu.Lock()
		svc.aliases["sshakku"] = existing
		svc.mu.Unlock()

		look, err := LookForCollection("sshakku", "sshakku", lookTestTimeout)
		if err != nil {
			t.Fatalf("LookForCollection: %v", err)
		}
		if !look.Running {
			t.Error("Running = false with a wallet on the bus")
		}
		if !look.CollectionFound {
			t.Error("CollectionFound = false for a compartment the alias resolves")
		}
	})

	t.Run("a wallet answering, with no compartment — and none is made", func(t *testing.T) {
		svc := startFakeOnBus(t)

		look, err := LookForCollection("sshakku", "sshakku", lookTestTimeout)
		if err != nil {
			t.Fatalf("LookForCollection: %v", err)
		}
		if !look.Running {
			t.Error("Running = false with a wallet on the bus")
		}
		if look.CollectionFound {
			t.Error("CollectionFound = true when the wallet holds no such compartment")
		}

		// The whole of what makes this a look rather than an act: resolving the
		// same names the ordinary way would have created what it did not find.
		svc.mu.Lock()
		made := len(svc.collections)
		svc.mu.Unlock()
		if made != 0 {
			t.Errorf("looking created %d collection(s); a report must leave the wallet as it found it", made)
		}
	})

	t.Run("a compartment an implementation keeps no alias for", func(t *testing.T) {
		svc := startFakeOnBus(t)
		svc.mu.Lock()
		svc.restrictAlias = true
		svc.mu.Unlock()
		if _, _, err := svc.CreateCollection(
			map[string]dbus.Variant{collectionLabelProp: dbus.MakeVariant("sshakku")}, ""); err != nil {
			t.Fatalf("seeding an unaliased collection: %v", err)
		}

		look, err := LookForCollection("sshakku", "sshakku", lookTestTimeout)
		if err != nil {
			t.Fatalf("LookForCollection: %v", err)
		}
		if !look.CollectionFound {
			t.Error("CollectionFound = false for a compartment carrying the name as its label")
		}
	})

	t.Run("a wallet that stopped answering", func(t *testing.T) {
		svc := startFakeOnBus(t)
		hang := make(chan struct{})
		t.Cleanup(func() { close(hang) })
		svc.hangOn(hang)

		started := time.Now()
		look, err := LookForCollection("sshakku", "sshakku", lookTestTimeout)
		elapsed := time.Since(started)

		if err != nil {
			t.Fatalf("LookForCollection: %v", err)
		}
		if !look.Running {
			t.Error("Running = false for a wallet that owns the name but does not reply")
		}
		if look.AskErr == nil {
			t.Error("AskErr = nil for a wallet that never answered; not answering is not the same as having nothing")
		}
		if look.CollectionFound {
			t.Error("CollectionFound = true was concluded from an answer that never came")
		}
		if elapsed > 4*lookTestTimeout {
			t.Errorf("the look took %s against a wallet that never answers, which is a wait with no bound", elapsed)
		}
	})

	t.Run("no compartment to ask about", func(t *testing.T) {
		svc := startFakeOnBus(t)
		hang := make(chan struct{})
		t.Cleanup(func() { close(hang) })
		// Armed so that asking about a compartment would be caught: this caller
		// wants only to know whether a wallet is there.
		svc.hangOn(hang)

		look, err := LookForCollection("", "", lookTestTimeout)
		if err != nil {
			t.Fatalf("LookForCollection: %v", err)
		}
		if !look.Running {
			t.Error("Running = false with a wallet on the bus")
		}
		if look.AskErr != nil {
			t.Errorf("AskErr = %v; nothing should have been asked of the wallet", look.AskErr)
		}
	})

	t.Run("a wallet that will not say what it holds", func(t *testing.T) {
		svc := startFakeOnBus(t)
		// Under the lock: the handler that reads these runs on the bus's own
		// goroutine, and nothing has been asked of it yet to order the two.
		svc.mu.Lock()
		svc.restrictAlias = true
		svc.failCollectionsProp = true
		svc.mu.Unlock()

		look, err := LookForCollection("sshakku", "sshakku", lookTestTimeout)
		if err != nil {
			t.Fatalf("LookForCollection: %v", err)
		}
		if look.AskErr == nil {
			t.Error("AskErr = nil for a wallet that refused to list what it holds")
		}
		if look.CollectionFound {
			t.Error("CollectionFound = true without an answer saying so")
		}
	})

	t.Run("no bus to look at", func(t *testing.T) {
		t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/nonexistent/bus")

		if _, err := LookForCollection("sshakku", "sshakku", lookTestTimeout); err == nil {
			t.Error("LookForCollection reported no error against a bus that is not there")
		}
	})

	t.Run("a bus with no wallet on it", func(t *testing.T) {
		startSessionBus(t)

		look, err := LookForCollection("sshakku", "sshakku", lookTestTimeout)
		if err != nil {
			t.Fatalf("LookForCollection: %v", err)
		}
		if look.Running {
			t.Error("Running = true on a bus nothing has claimed the name on")
		}
		if look.AskErr != nil {
			t.Errorf("AskErr = %v; there was nothing there to ask", look.AskErr)
		}
	})
}
