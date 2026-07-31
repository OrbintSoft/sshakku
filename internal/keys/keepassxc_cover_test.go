package keys

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OrbintSoft/sshakku/internal/keepassxc"
)

func TestUnavailableBackendFailsEveryOperationWithItsReason(t *testing.T) {
	reason := errors.New("the secret-service route needs an API this platform has none of")
	b := UnavailableBackend{Reason: reason}

	if _, _, err := b.Lookup("k"); !errors.Is(err, reason) {
		t.Errorf("Lookup err = %v, want the reason — a miss would send the loader to prompt with no explanation", err)
	}
	if err := b.Store("k", "k", "p"); !errors.Is(err, reason) {
		t.Errorf("Store err = %v, want the reason", err)
	}
	if err := b.Delete("k"); !errors.Is(err, reason) {
		t.Errorf("Delete err = %v, want the reason", err)
	}
	if _, err := b.List(); !errors.Is(err, reason) {
		t.Errorf("List err = %v, want the reason", err)
	}
}

// TestUnavailableBackendLookupIsNotAMiss states the distinction the type
// exists for: reporting "nothing stored" would let a later store overwrite
// whatever is really in the wallet.
func TestUnavailableBackendLookupIsNotAMiss(t *testing.T) {
	_, found, err := UnavailableBackend{Reason: errors.New("unreachable")}.Lookup("k")
	if found {
		t.Error("an unreachable store must not claim to have looked and found nothing")
	}
	if err == nil {
		t.Error("an unreachable store must report why, not stay silent")
	}
}

func TestDialKeePassXCNamesEveryPathItTried(t *testing.T) {
	absent := []string{
		filepath.Join(shortDir(t), "a"),
		filepath.Join(shortDir(t), "b"),
	}
	_, err := dialKeePassXCAt(absent)
	if err == nil {
		t.Fatal("no socket answering must be an error")
	}
	if !errors.Is(err, keepassxc.ErrNotRunning) {
		t.Errorf("err = %v, want ErrNotRunning", err)
	}
	for _, path := range absent {
		if !strings.Contains(err.Error(), path) {
			t.Errorf("err = %v, want it to name %s — otherwise the user cannot tell where it looked", err, path)
		}
	}
}

func TestDialKeePassXCTakesTheFirstThatAnswers(t *testing.T) {
	dir := shortDir(t)
	live := filepath.Join(dir, "live")
	ln, err := net.Listen("unix", live)
	if err != nil {
		t.Fatalf("listening: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr == nil {
			_ = conn.Close()
		}
	}()

	conn, err := dialKeePassXCAt([]string{filepath.Join(dir, "absent"), live})
	if err != nil {
		t.Fatalf("a later candidate that answers must be used: %v", err)
	}
	_ = conn.Close()
}

func TestKeePassXCBackendDefaultsToThePlatformPaths(t *testing.T) {
	b := KeePassXCBackend{}
	if len(b.socketPaths()) == 0 {
		t.Fatal("a backend that configured no paths must still have somewhere to look")
	}
	configured := KeePassXCBackend{SocketPaths: []string{"/somewhere/else"}}
	got := configured.socketPaths()
	if len(got) != 1 || got[0] != "/somewhere/else" {
		t.Errorf("socketPaths = %v, want exactly what was configured", got)
	}
}

// TestKeePassXCConnectReportsAnUnreachableSocket drives the real construction
// path — no session seam — against paths that cannot answer.
func TestKeePassXCConnectReportsAnUnreachableSocket(t *testing.T) {
	b := KeePassXCBackend{
		SocketPaths:  []string{filepath.Join(shortDir(t), "absent")},
		Associations: &memoryAssociations{},
	}
	if _, _, err := b.Lookup("id_ed25519"); !errors.Is(err, keepassxc.ErrNotRunning) {
		t.Fatalf("err = %v, want ErrNotRunning", err)
	}
}

// TestKeePassXCConnectReportsAFailedHandshake drives the same path against a
// socket that accepts and then says nothing, which is not the same as nothing
// listening.
func TestKeePassXCConnectReportsAFailedHandshake(t *testing.T) {
	path := filepath.Join(shortDir(t), "mute")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listening: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr != nil {
			return
		}
		// Accept, then hang up without a word.
		_ = conn.Close()
	}()

	b := KeePassXCBackend{
		SocketPaths:  []string{path},
		Associations: &memoryAssociations{},
		Timeout:      2 * time.Second,
	}
	if _, _, err := b.Lookup("id_ed25519"); err == nil {
		t.Fatal("a socket that accepts and says nothing must fail the lookup, not hang")
	}
}

func TestKeePassXCLookupReportsARevokedAssociation(t *testing.T) {
	kp := &fakeKeePassXC{testAssociateErr: keepassxc.ErrNotAssociated}
	b := kp.backendFor(&memoryAssociations{stored: keepassxc.Association{ID: "db", IDKey: "k"}, present: true})

	if _, _, err := b.Lookup("id_ed25519"); !errors.Is(err, keepassxc.ErrNotAssociated) {
		t.Fatalf("err = %v, want ErrNotAssociated — an approval the user revoked is not a miss", err)
	}
}

func TestKeePassXCLookupReportsAFailedSearch(t *testing.T) {
	kp := &fakeKeePassXC{getLoginsErr: keepassxc.ErrDatabaseLocked}
	b := kp.backendFor(&memoryAssociations{stored: keepassxc.Association{ID: "db", IDKey: "k"}, present: true})

	if _, _, err := b.Lookup("id_ed25519"); !errors.Is(err, keepassxc.ErrDatabaseLocked) {
		t.Fatalf("err = %v, want ErrDatabaseLocked — a locked database is not an empty one", err)
	}
}

func TestKeePassXCStoreReportsARefusedApproval(t *testing.T) {
	refused := errors.New("the user closed the dialog")
	kp := &fakeKeePassXC{associateErr: refused}
	b := kp.backendFor(&memoryAssociations{})

	if err := b.Store("id_ed25519", "", "p"); !errors.Is(err, refused) {
		t.Fatalf("err = %v, want the refusal", err)
	}
}

// TestKeePassXCStoreCreatesWhenTheSearchFails proves a store still lands when
// the lookup for an existing entry could not run: without a uuid it creates one
// rather than giving up on saving the passphrase.
func TestKeePassXCStoreCreatesWhenTheSearchFails(t *testing.T) {
	kp := &fakeKeePassXC{getLoginsErr: errors.New("search unavailable")}
	b := kp.backendFor(&memoryAssociations{stored: keepassxc.Association{ID: "db", IDKey: "k"}, present: true})

	if err := b.Store("id_ed25519", "", "p"); err != nil {
		t.Fatalf("Store: %v", err)
	}
	if !kp.lastSet.called {
		t.Fatal("the passphrase must still be stored")
	}
	if kp.lastSet.uuid != "" {
		t.Errorf("uuid = %q, want empty so KeePassXC creates the entry", kp.lastSet.uuid)
	}
}

func TestKeePassXCStoreReportsAFailedWrite(t *testing.T) {
	kp := &fakeKeePassXC{setLoginErr: errors.New("read-only database")}
	b := kp.backendFor(&memoryAssociations{stored: keepassxc.Association{ID: "db", IDKey: "k"}, present: true})

	if err := b.Store("id_ed25519", "", "p"); err == nil {
		t.Fatal("a passphrase that could not be written must be reported")
	}
}

func TestKeePassXCStoreEntersTheSSHakkuGroup(t *testing.T) {
	kp := &fakeKeePassXC{}
	b := kp.backendFor(&memoryAssociations{stored: keepassxc.Association{ID: "db", IDKey: "k"}, present: true})

	if err := b.Store("id_ed25519", "", "p"); err != nil {
		t.Fatalf("Store: %v", err)
	}
	if kp.lastSet.group != "SSHakku" {
		t.Errorf("group = %q, want the entries kept together where a user can find them", kp.lastSet.group)
	}
}

// TestKeePassXCConnectSucceedsOverARealSocket drives the production path with
// no session seam: a socket that answers the key exchange the way KeePassXC
// does. Only the exchange is answered, so the lookup then stops at "not
// associated" — which is the point: the connection itself was built for real.
func TestKeePassXCConnectSucceedsOverARealSocket(t *testing.T) {
	path := filepath.Join(shortDir(t), "s")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listening: %v", err)
	}
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

	b := KeePassXCBackend{
		SocketPaths:  []string{path},
		Associations: &memoryAssociations{},
		Timeout:      5 * time.Second,
	}
	_, _, err = b.Lookup("id_ed25519")
	if !errors.Is(err, keepassxc.ErrNotAssociated) {
		t.Fatalf("err = %v, want ErrNotAssociated — the session opened, and nothing was ever approved", err)
	}
}
