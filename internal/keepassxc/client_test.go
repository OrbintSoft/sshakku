package keepassxc

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestConnectExchangesKeys(t *testing.T) {
	s := newFakeServer(t)
	c := s.connect()

	if c.hostKey == nil {
		t.Fatal("Connect must record the host public key")
	}
	if *c.hostKey != *s.keys.public {
		t.Error("the recorded host key is not the one the server sent")
	}
	if c.clientID == "" {
		t.Error("Connect must generate a client id")
	}
}

func TestConnectRejectsAKeyOfTheWrongWidth(t *testing.T) {
	s := newFakeServer(t)
	s.prelude[actionChangePublicKeys] = []envelope{{
		Action:    actionChangePublicKeys,
		PublicKey: "c2hvcnQ=", // "short"
		Success:   "true",
	}}

	_, err := Connect(s.dial(), 2*time.Second, 2*time.Second)
	if err == nil {
		t.Fatal("a public key of the wrong width must be refused")
	}
	if !strings.Contains(err.Error(), "32 bytes") {
		t.Errorf("error = %v, want it to name the expected width", err)
	}
}

func TestAssociateReturnsTheDatabaseIdAndIdentificationKey(t *testing.T) {
	s := newFakeServer(t)
	s.reply = func(action string, _ map[string]any) any {
		if action != actionAssociate {
			return nil
		}
		return map[string]any{"success": "true", "id": "sshakku-db", "hash": "abc"}
	}
	c := s.connect()

	got, err := c.Associate()
	if err != nil {
		t.Fatalf("Associate: %v", err)
	}
	if got.ID != "sshakku-db" {
		t.Errorf("ID = %q, want sshakku-db", got.ID)
	}
	if got.IDKey == "" {
		t.Error("Associate must return the identification key it registered")
	}

	// The key returned must be the one actually sent, or a later
	// test-associate would present a key KeePassXC never saw.
	opened := s.openedPayloads()
	if len(opened) != 1 || opened[0]["idKey"] != got.IDKey {
		t.Errorf("the returned idKey is not the one sent on the wire: %v", opened)
	}
}

func TestAssociateWithoutADatabaseIdIsAnError(t *testing.T) {
	s := newFakeServer(t)
	s.reply = func(string, map[string]any) any {
		return map[string]any{"success": "true"}
	}
	c := s.connect()

	if _, err := c.Associate(); err == nil {
		t.Fatal("an association naming no database must not be reported as success")
	}
}

// The error codes below are written as the literals KeePassXC puts on the wire,
// not as this package's constants. Using the constant would make the test agree
// with the code by construction: changing the constant to a wrong value would
// change both sides at once and the test could never catch it.
func TestTestAssociateRefusedReportsNotAssociated(t *testing.T) {
	s := newFakeServer(t)
	s.failWith[actionTestAssociate] = envelope{Success: "false", Code: "4"}
	c := s.connect()

	err := c.TestAssociate(Association{ID: "gone", IDKey: "key"})
	if !errors.Is(err, ErrNotAssociated) {
		t.Fatalf("err = %v, want ErrNotAssociated — the caller decides to associate again from this", err)
	}
}

func TestLockedDatabaseIsItsOwnError(t *testing.T) {
	s := newFakeServer(t)
	s.failWith[actionGetLogins] = envelope{Success: "false", Code: "1"}
	c := s.connect()

	_, err := c.GetLogins("ssh://id_test", Association{ID: "db", IDKey: "key"})
	if !errors.Is(err, ErrDatabaseLocked) {
		t.Fatalf("err = %v, want ErrDatabaseLocked", err)
	}
}

func TestGetLoginsReturnsTheStoredEntry(t *testing.T) {
	s := newFakeServer(t)
	s.reply = func(action string, _ map[string]any) any {
		if action != actionGetLogins {
			return nil
		}
		return map[string]any{
			"success": "true",
			"count":   1,
			"entries": []map[string]string{
				{"login": "id_test", "name": "id_test", "password": "test-passphrase", "uuid": "u-1"},
			},
		}
	}
	c := s.connect()

	entries, err := c.GetLogins("ssh://id_test", Association{ID: "db", IDKey: "key"})
	if err != nil {
		t.Fatalf("GetLogins: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].Password != "test-passphrase" {
		t.Errorf("password = %q, want test-passphrase", entries[0].Password)
	}
	if entries[0].UUID != "u-1" {
		t.Errorf("uuid = %q, want u-1 — it is what lets a later store update in place", entries[0].UUID)
	}
}

func TestGetLoginsNoMatchIsEmptyNotAnError(t *testing.T) {
	s := newFakeServer(t)
	s.reply = func(string, map[string]any) any {
		return map[string]any{"success": "true", "count": 0, "entries": []map[string]string{}}
	}
	c := s.connect()

	entries, err := c.GetLogins("ssh://absent", Association{ID: "db", IDKey: "key"})
	if err != nil {
		t.Fatalf("a miss must not be an error, got %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("got %d entries, want none", len(entries))
	}
}

// TestSetLoginNeverPutsThePassphraseInTheClear is the assertion the whole
// encryption layer exists for. It reads the raw bytes the client wrote, not the
// decrypted payload: the point is that anything reading the socket sees
// ciphertext.
func TestSetLoginNeverPutsThePassphraseInTheClear(t *testing.T) {
	const passphrase = "correct-horse-battery-staple"
	s := newFakeServer(t)
	c := s.connect()

	if err := c.SetLogin("ssh://id_test", "id_test", passphrase, "", "SSHakku", Association{ID: "db", IDKey: "key"}); err != nil {
		t.Fatalf("SetLogin: %v", err)
	}

	// First prove the round trip works, so the absence below is not just an
	// absence of anything: the server did receive the passphrase, decrypted.
	opened := s.openedPayloads()
	if len(opened) != 1 || opened[0]["password"] != passphrase {
		t.Fatalf("the server decrypted %v, want the passphrase — the round trip is broken", opened)
	}

	wire := s.rawBytes()
	if len(wire) == 0 {
		t.Fatal("nothing was recorded off the socket, so the assertion below would prove nothing")
	}
	if bytes.Contains(wire, []byte(passphrase)) {
		t.Fatal("the passphrase crossed the socket in the clear")
	}
}

// TestUnsolicitedFrameIsSkipped drives the case KeePassXC creates on its own:
// it announces a lock or unlock whenever it happens, so those frames arrive
// interleaved with replies rather than only when asked for.
func TestUnsolicitedFrameIsSkipped(t *testing.T) {
	s := newFakeServer(t)
	s.prelude[actionGetLogins] = []envelope{
		{Action: "database-locked"},
		{Action: "database-unlocked"},
	}
	s.reply = func(string, map[string]any) any {
		return map[string]any{
			"success": "true",
			"entries": []map[string]string{{"login": "id_test", "password": "p"}},
		}
	}
	c := s.connect()

	entries, err := c.GetLogins("ssh://id_test", Association{ID: "db", IDKey: "key"})
	if err != nil {
		t.Fatalf("an unsolicited frame must be skipped, not mistaken for the reply: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
}

// TestReplyUnderTheWrongNonceIsRefused proves the nonce is checked rather than
// carried along. Without it a reply produced for some other request — replayed,
// or answered out of order — would be accepted as this one's answer.
func TestReplyUnderTheWrongNonceIsRefused(t *testing.T) {
	s := newFakeServer(t)
	s.sealUnderWrongNonce = true
	s.reply = func(string, map[string]any) any {
		return map[string]any{"success": "true", "id": "db"}
	}
	c := s.connect()

	if _, err := c.Associate(); err == nil {
		t.Fatal("a reply sealed under a different nonce must not decrypt")
	}
}

func TestIncrementNonce(t *testing.T) {
	tests := []struct {
		name string
		in   func() [nonceLen]byte
		want func() [nonceLen]byte
	}{
		{
			name: "zero becomes one",
			in:   func() (n [nonceLen]byte) { return },
			want: func() (n [nonceLen]byte) { n[0] = 1; return },
		},
		{
			name: "carries into the next byte",
			in:   func() (n [nonceLen]byte) { n[0] = 255; return },
			want: func() (n [nonceLen]byte) { n[1] = 1; return },
		},
		{
			name: "carries across every byte and wraps",
			in: func() (n [nonceLen]byte) {
				for i := range n {
					n[i] = 255
				}
				return
			},
			want: func() (n [nonceLen]byte) { return },
		},
		{
			name: "leaves the high bytes alone",
			in:   func() (n [nonceLen]byte) { n[0] = 7; n[23] = 9; return },
			want: func() (n [nonceLen]byte) { n[0] = 8; n[23] = 9; return },
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got, want := incrementNonce(tc.in()), tc.want(); got != want {
				t.Errorf("incrementNonce = %v, want %v", got, want)
			}
		})
	}
}

func TestRequestBeforeKeyExchangeIsRefused(t *testing.T) {
	c := &Client{timeout: time.Second}
	if err := c.request(actionGetLogins, struct{}{}, &getLoginsReply{}); err == nil {
		t.Fatal("a request before the key exchange must not be attempted")
	}
}
