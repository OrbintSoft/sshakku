package wire

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnectExchangesKeys(t *testing.T) {
	s := newFakeServer(t)
	c := s.connect()

	require.NotNil(t, c.hostKey, "Connect must record the host public key")
	assert.Equal(t, *s.keys.public, *c.hostKey, "the recorded host key must be the one the server sent")
	assert.NotEmpty(t, c.clientID, "Connect must generate a client id")
}

func TestConnectRejectsAKeyOfTheWrongWidth(t *testing.T) {
	s := newFakeServer(t)
	s.prelude[actionChangePublicKeys] = []envelope{{
		Action:    actionChangePublicKeys,
		PublicKey: "c2hvcnQ=", // "short"
		Success:   "true",
	}}

	_, err := Connect(s.dial(), 2*time.Second, 2*time.Second)
	require.Error(t, err, "a public key of the wrong width must be refused")
	assert.ErrorContains(t, err, "32 bytes", "the error must name the expected width")
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
	require.NoError(t, err, "Associate")
	assert.Equal(t, "sshakku-db", got.ID, "ID")
	assert.NotEmpty(t, got.IDKey, "Associate must return the identification key it registered")

	// The key returned must be the one actually sent, or a later
	// test-associate would present a key KeePassXC never saw.
	opened := s.openedPayloads()
	require.Len(t, opened, 1, "exactly one payload must have been sent")
	assert.Equal(t, got.IDKey, opened[0]["idKey"], "the returned idKey must be the one sent on the wire")
}

func TestAssociateWithoutADatabaseIdIsAnError(t *testing.T) {
	s := newFakeServer(t)
	s.reply = func(string, map[string]any) any {
		return map[string]any{"success": "true"}
	}
	c := s.connect()

	_, err := c.Associate()
	assert.Error(t, err, "an association naming no database must not be reported as success")
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
	assert.ErrorIs(t, err, ErrNotAssociated, "the caller decides to associate again from this")
}

func TestLockedDatabaseIsItsOwnError(t *testing.T) {
	s := newFakeServer(t)
	s.failWith[actionGetLogins] = envelope{Success: "false", Code: "1"}
	c := s.connect()

	_, err := c.GetLogins("ssh://id_test", Association{ID: "db", IDKey: "key"})
	assert.ErrorIs(t, err, ErrDatabaseLocked, "GetLogins against a locked database")
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
	require.NoError(t, err, "GetLogins")
	require.Len(t, entries, 1, "entries")
	assert.Equal(t, "test-passphrase", entries[0].Password, "password")
	assert.Equal(t, "u-1", entries[0].UUID, "uuid — it is what lets a later store update in place")
}

func TestGetLoginsNoMatchIsEmptyNotAnError(t *testing.T) {
	s := newFakeServer(t)
	s.reply = func(string, map[string]any) any {
		return map[string]any{"success": "true", "count": 0, "entries": []map[string]string{}}
	}
	c := s.connect()

	entries, err := c.GetLogins("ssh://absent", Association{ID: "db", IDKey: "key"})
	require.NoError(t, err, "a miss must not be an error")
	assert.Empty(t, entries, "entries")
}

// TestSetLoginNeverPutsThePassphraseInTheClear is the assertion the whole
// encryption layer exists for. It reads the raw bytes the client wrote, not the
// decrypted payload: the point is that anything reading the socket sees
// ciphertext.
func TestSetLoginNeverPutsThePassphraseInTheClear(t *testing.T) {
	const passphrase = "correct-horse-battery-staple"
	s := newFakeServer(t)
	c := s.connect()

	require.NoError(t,
		c.SetLogin("ssh://id_test", "id_test", passphrase, "", "SSHakku", Association{ID: "db", IDKey: "key"}),
		"SetLogin")

	// First prove the round trip works, so the absence below is not just an
	// absence of anything: the server did receive the passphrase, decrypted.
	opened := s.openedPayloads()
	require.Len(t, opened, 1, "the server must have opened exactly one payload")
	require.Equal(t, passphrase, opened[0]["password"], "the server must have decrypted the passphrase — otherwise the round trip is broken")

	wire := s.rawBytes()
	require.NotEmpty(t, wire, "nothing was recorded off the socket, so the assertion below would prove nothing")
	assert.NotContains(t, string(wire), passphrase, "the passphrase must not cross the socket in the clear")
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
	require.NoError(t, err, "an unsolicited frame must be skipped, not mistaken for the reply")
	assert.Len(t, entries, 1, "entries")
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

	_, err := c.Associate()
	assert.Error(t, err, "a reply sealed under a different nonce must not decrypt")
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
			assert.Equal(t, tc.want(), incrementNonce(tc.in()), "incrementNonce")
		})
	}
}

func TestRequestBeforeKeyExchangeIsRefused(t *testing.T) {
	c := &Client{timeout: time.Second}
	err := c.request(actionGetLogins, struct{}{}, &getLoginsReply{})
	assert.Error(t, err, "a request before the key exchange must not be attempted")
}
