package keepassxc

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/OrbintSoft/sshakku/internal/keys/wallet/keepassxc/wire"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/testtmp"
)

// The failures these tests hand their seams. Each stands for a real one the
// code under test cannot be made to produce on demand.
var (
	errReadOnlyDatabase       = errors.New("read-only database")
	errSearchUnavailable      = errors.New("search unavailable")
	errTheUserClosedTheDialog = errors.New("the user closed the dialog")
)

func TestDialKeePassXCNamesEveryPathItTried(t *testing.T) {
	absent := []string{
		filepath.Join(testtmp.ShortDir(t), "a"),
		filepath.Join(testtmp.ShortDir(t), "b"),
	}
	_, err := dialKeePassXCAt(t.Context(), absent)
	require.Error(t, err, "a KeePassXC nothing could reach cannot answer")
	assert.ErrorIs(t, err, wire.ErrNotRunning, "and it must be said as an app that is not running")
	for _, path := range absent {
		assert.Containsf(t, err.Error(), path,
			"naming every place it looked, or the user cannot tell where to put the socket they do have")
	}
}

func TestDialKeePassXCTakesTheFirstThatAnswers(t *testing.T) {
	dir := testtmp.ShortDir(t)
	live := filepath.Join(dir, "live")
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", live)
	require.NoError(t, err, "a socket that answers, later in the list than one that does not")
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr == nil {
			_ = conn.Close()
		}
	}()

	conn, err := dialKeePassXCAt(t.Context(), []string{filepath.Join(dir, "absent"), live})
	require.NoError(t, err,
		"the first candidate not answering must not end the search: KeePassXC's socket moved between versions")
	_ = conn.Close()
}

func TestKeePassXCBackendDefaultsToThePlatformPaths(t *testing.T) {
	b := Native{}
	assert.NotEmpty(t, b.socketPaths(), "a route that configured no paths must still have the usual places to look")
	configured := Native{SocketPaths: []string{"/somewhere/else"}}
	assert.Equal(t, []string{"/somewhere/else"}, configured.socketPaths(),
		"and one that named its own must be looked for there and nowhere else")
}

// TestKeePassXCConnectReportsAnUnreachableSocket drives the real construction
// path — no session seam — against paths that cannot answer.
func TestKeePassXCConnectReportsAnUnreachableSocket(t *testing.T) {
	b := Native{
		SocketPaths:  []string{filepath.Join(testtmp.ShortDir(t), "absent")},
		Associations: &memoryAssociations{},
	}
	_, _, err := b.Lookup(t.Context(), "id_ed25519")
	assert.ErrorIs(t, err, wire.ErrNotRunning,
		"a KeePassXC that is not running must be said so, not read as a database holding nothing")
}

// TestKeePassXCConnectReportsAFailedHandshake drives the same path against a
// socket that accepts and then says nothing, which is not the same as nothing
// listening.
func TestKeePassXCConnectReportsAFailedHandshake(t *testing.T) {
	path := filepath.Join(testtmp.ShortDir(t), "mute")
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", path)
	require.NoError(t, err, "a socket that accepts and then says nothing")
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr != nil {
			return
		}
		// Accept, then hang up without a word.
		_ = conn.Close()
	}()

	b := Native{
		SocketPaths:  []string{path},
		Associations: &memoryAssociations{},
		Timeout:      2 * time.Second,
	}
	_, _, err = b.Lookup(t.Context(), "id_ed25519")
	assert.Error(t, err,
		"a socket that accepts and says nothing is not the same as nothing listening, and must not hold the caller")
}

func TestKeePassXCLookupReportsARevokedAssociation(t *testing.T) {
	kp := &fakeKeePassXC{testAssociateErr: wire.ErrNotAssociated}
	b := kp.backendFor(&memoryAssociations{stored: wire.Association{ID: "db", IDKey: "k"}, present: true})

	_, _, err := b.Lookup(t.Context(), "id_ed25519")
	assert.ErrorIs(t, err, wire.ErrNotAssociated,
		"an approval the user revoked must be said so, not read as a database holding nothing")
}

func TestKeePassXCLookupReportsAFailedSearch(t *testing.T) {
	kp := &fakeKeePassXC{getLoginsErr: wire.ErrDatabaseLocked}
	b := kp.backendFor(&memoryAssociations{stored: wire.Association{ID: "db", IDKey: "k"}, present: true})

	_, _, err := b.Lookup(t.Context(), "id_ed25519")
	assert.ErrorIs(t, err, wire.ErrDatabaseLocked,
		"a locked database is not an empty one, and telling them apart is what sends the user to unlock it")
}

func TestKeePassXCStoreReportsARefusedApproval(t *testing.T) {
	refused := errTheUserClosedTheDialog
	kp := &fakeKeePassXC{associateErr: refused}
	b := kp.backendFor(&memoryAssociations{})

	assert.ErrorIs(t, b.Store(t.Context(), "id_ed25519", "", "p"), refused,
		"a user who closed the approval dialog has answered, and must be obeyed rather than worked around")
}

// TestKeePassXCStoreCreatesWhenTheSearchFails proves a store still lands when
// the lookup for an existing entry could not run: without a uuid it creates one
// rather than giving up on saving the passphrase.
func TestKeePassXCStoreCreatesWhenTheSearchFails(t *testing.T) {
	kp := &fakeKeePassXC{getLoginsErr: errSearchUnavailable}
	b := kp.backendFor(&memoryAssociations{stored: wire.Association{ID: "db", IDKey: "k"}, present: true})

	require.NoError(t, b.Store(t.Context(), "id_ed25519", "", "p"),
		"not knowing whether an entry is already there is no reason to lose the passphrase the user just typed")
	assert.True(t, kp.lastSet.called, "so it must still be written")
	assert.Empty(t, kp.lastSet.uuid, "naming no existing entry, so KeePassXC creates one rather than replacing a guess")
}

func TestKeePassXCStoreReportsAFailedWrite(t *testing.T) {
	kp := &fakeKeePassXC{setLoginErr: errReadOnlyDatabase}
	b := kp.backendFor(&memoryAssociations{stored: wire.Association{ID: "db", IDKey: "k"}, present: true})

	assert.Error(t, b.Store(t.Context(), "id_ed25519", "", "p"),
		"a passphrase the database refused to take must not be reported as saved: the next login would expect it there")
}

func TestKeePassXCStoreEntersTheSSHakkuGroup(t *testing.T) {
	kp := &fakeKeePassXC{}
	b := kp.backendFor(&memoryAssociations{stored: wire.Association{ID: "db", IDKey: "k"}, present: true})

	require.NoError(t, b.Store(t.Context(), "id_ed25519", "", "p"), "saving a passphrase must succeed")
	assert.Equal(t, "SSHakku", kp.lastSet.group,
		"SSHakku's entries belong together, where a person browsing their own database can find them")
}

// TestKeePassXCConnectSucceedsOverARealSocket drives the production path with
// no session seam: a socket that answers the key exchange the way KeePassXC
// does. Only the exchange is answered, so the lookup then stops at "not
// associated" — which is the point: the connection itself was built for real.
func TestKeePassXCConnectSucceedsOverARealSocket(t *testing.T) {
	path := filepath.Join(testtmp.ShortDir(t), "s")
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", path)
	require.NoError(t, err, "a socket that answers the key exchange the way KeePassXC does")
	t.Cleanup(func() { _ = ln.Close() })

	// A public key of the right width is all the exchange needs to complete.
	var hostKey [32]byte
	for i := range hostKey {
		hostKey[i] = byte(i + 1)
	}
	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		var req map[string]any
		if decodeErr := json.NewDecoder(conn).Decode(&req); decodeErr != nil {
			return
		}
		reply, marshalErr := json.Marshal(map[string]string{
			"action":    "change-public-keys",
			"publicKey": base64.StdEncoding.EncodeToString(hostKey[:]),
			"success":   "true",
		})
		if marshalErr != nil {
			return
		}
		_, _ = conn.Write(reply)
	}()

	b := Native{
		SocketPaths:  []string{path},
		Associations: &memoryAssociations{},
		Timeout:      5 * time.Second,
	}
	_, _, err = b.Lookup(t.Context(), "id_ed25519")
	assert.ErrorIs(t, err, wire.ErrNotAssociated,
		"the session opened for real over a real socket, and stopped where it should: nothing was ever approved")
}
