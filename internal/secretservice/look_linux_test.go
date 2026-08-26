//go:build linux

package secretservice

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errTheBusStoppedAnswering is the failure this test hands its seam, standing for a real one the
// code under test cannot be made to produce on demand.
var errTheBusStoppedAnswering = errors.New("the bus stopped answering")

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
	require.NoError(t, err, "server connect")
	t.Cleanup(func() { _ = serverConn.Close() })
	return startFakeSecretService(t, serverConn, "")
}

// failBusNames makes the message bus stop answering the named question for the
// rest of the test, and go on answering the others. Only the bus's own name
// lists are replaced: what the look then concludes is left to the code under
// test.
func failBusNames(t *testing.T, method string) {
	t.Helper()
	original := listBusNames
	t.Cleanup(func() { listBusNames = original })

	listBusNames = func(ctx context.Context, obj dbus.BusObject, timeout time.Duration, called string) ([]string, error) {
		if strings.HasSuffix(called, method) {
			return nil, errTheBusStoppedAnswering
		}
		return original(ctx, obj, timeout, called)
	}
}

func TestLookForCollection(t *testing.T) {
	t.Run("a wallet answering, with the compartment already there", func(t *testing.T) {
		svc := startFakeOnBus(t)
		const existing = dbus.ObjectPath("/org/freedesktop/secrets/collection/existing")
		svc.mu.Lock()
		svc.aliases["sshakku"] = existing
		svc.mu.Unlock()

		look, err := LookForCollection(t.Context(), "sshakku", "sshakku", lookTestTimeout)
		require.NoError(t, err, "LookForCollection")
		assert.True(t, look.Running, "a wallet is on the bus")
		assert.True(t, look.CollectionFound, "the alias resolves the compartment")
	})

	t.Run("a wallet answering, with no compartment — and none is made", func(t *testing.T) {
		svc := startFakeOnBus(t)

		look, err := LookForCollection(t.Context(), "sshakku", "sshakku", lookTestTimeout)
		require.NoError(t, err, "LookForCollection")
		assert.True(t, look.Running, "a wallet is on the bus")
		assert.False(t, look.CollectionFound, "the wallet holds no such compartment")

		// The whole of what makes this a look rather than an act: resolving the
		// same names the ordinary way would have created what it did not find.
		svc.mu.Lock()
		made := len(svc.collections)
		svc.mu.Unlock()
		assert.Zero(t, made, "a report must leave the wallet as it found it")
	})

	t.Run("a compartment an implementation keeps no alias for", func(t *testing.T) {
		svc := startFakeOnBus(t)
		svc.mu.Lock()
		svc.restrictAlias = true
		svc.mu.Unlock()
		_, _, dbusErr := svc.CreateCollection(
			map[string]dbus.Variant{collectionLabelProp: dbus.MakeVariant("sshakku")}, "")
		require.Nil(t, dbusErr, "seeding an unaliased collection")

		look, err := LookForCollection(t.Context(), "sshakku", "sshakku", lookTestTimeout)
		require.NoError(t, err, "LookForCollection")
		assert.True(t, look.CollectionFound, "the compartment carries the name as its label")
	})

	t.Run("a wallet that stopped answering", func(t *testing.T) {
		svc := startFakeOnBus(t)
		hang := make(chan struct{})
		t.Cleanup(func() { close(hang) })
		svc.hangOn(hang)

		started := time.Now()
		look, err := LookForCollection(t.Context(), "sshakku", "sshakku", lookTestTimeout)
		elapsed := time.Since(started)

		require.NoError(t, err, "LookForCollection")
		assert.True(t, look.Running, "the wallet owns the name even though it does not reply")
		assert.Error(t, look.AskErr, "not answering is not the same as having nothing")
		assert.False(t, look.CollectionFound, "nothing may be concluded from an answer that never came")
		assert.Less(t, elapsed, 4*lookTestTimeout, "the look must be bounded against a wallet that never answers")
	})

	t.Run("no compartment to ask about", func(t *testing.T) {
		svc := startFakeOnBus(t)
		hang := make(chan struct{})
		t.Cleanup(func() { close(hang) })
		// Armed so that asking about a compartment would be caught: this caller
		// wants only to know whether a wallet is there.
		svc.hangOn(hang)

		look, err := LookForCollection(t.Context(), "", "", lookTestTimeout)
		require.NoError(t, err, "LookForCollection")
		assert.True(t, look.Running, "a wallet is on the bus")
		assert.NoError(t, look.AskErr, "nothing should have been asked of the wallet")
	})

	t.Run("a wallet that will not say what it holds", func(t *testing.T) {
		svc := startFakeOnBus(t)
		// Under the lock: the handler that reads these runs on the bus's own
		// goroutine, and nothing has been asked of it yet to order the two.
		svc.mu.Lock()
		svc.restrictAlias = true
		svc.failCollectionsProp = true
		svc.mu.Unlock()

		look, err := LookForCollection(t.Context(), "sshakku", "sshakku", lookTestTimeout)
		require.NoError(t, err, "LookForCollection")
		assert.Error(t, look.AskErr, "a wallet that refused to list what it holds must be reported")
		assert.False(t, look.CollectionFound, "nothing may be concluded without an answer saying so")
	})

	t.Run("a bus that will not say what is on it", func(t *testing.T) {
		startFakeOnBus(t)
		failBusNames(t, "ListNames")

		_, err := LookForCollection(t.Context(), "sshakku", "sshakku", lookTestTimeout)
		assert.Error(t, err, "a bus that never said what was on it must be reported")
	})

	t.Run("a bus that will not say what could be started", func(t *testing.T) {
		startSessionBus(t)
		failBusNames(t, "ListActivatableNames")

		look, err := LookForCollection(t.Context(), "sshakku", "sshakku", lookTestTimeout)
		assert.Error(t, err, "a bus that never answered must be reported")
		assert.False(t, look.Activatable, "nothing may be concluded from a list that never arrived")
	})

	t.Run("no bus to look at", func(t *testing.T) {
		t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/nonexistent/bus")

		_, err := LookForCollection(t.Context(), "sshakku", "sshakku", lookTestTimeout)
		assert.Error(t, err, "a bus that is not there must be reported")
	})

	t.Run("a bus with no wallet on it", func(t *testing.T) {
		startSessionBus(t)

		look, err := LookForCollection(t.Context(), "sshakku", "sshakku", lookTestTimeout)
		require.NoError(t, err, "LookForCollection")
		assert.False(t, look.Running, "nothing has claimed the name on this bus")
		assert.False(t, look.Activatable, "this bus knows how to start no wallet either")
		assert.NoError(t, look.AskErr, "there was nothing there to ask")
	})

	t.Run("a bus with a wallet it could start", func(t *testing.T) {
		startActivatableSessionBus(t)

		look, err := LookForCollection(t.Context(), "sshakku", "sshakku", lookTestTimeout)
		require.NoError(t, err, "LookForCollection")
		assert.False(t, look.Running, "nothing has claimed the name yet")
		assert.True(t, look.Activatable, "the bus knows how to start a wallet")
		// Starting it would be an act, and this is a look. The wallet this bus
		// would start never claims the name, so a look that asked it anything
		// would have started it, been left without an answer, and reported that
		// instead of the two lines above.
		assert.False(t, look.CollectionFound, "nothing may be concluded about a wallet that was not started")
		assert.NoError(t, look.AskErr, "a wallet that was left alone was not asked")
	})
}
