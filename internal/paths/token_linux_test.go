//go:build linux

package paths

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/keyring"
)

// The failures these tests hand their seams. Each stands for a real one the
// code under test cannot be made to produce on demand.
var (
	errAddDenied  = errors.New("add denied")
	errNoEntropy  = errors.New("no entropy")
	errReadDenied = errors.New("read denied")
)

// saveTokenSeams snapshots the keyring/RNG seams and restores them when the
// (sub)test ends, so a test can swap them without leaking into its siblings.
func saveTokenSeams(t *testing.T) {
	t.Helper()
	origRand, origAdd, origSearch, origRead := randRead, keyringAdd, keyringSearch, keyringRead
	t.Cleanup(func() {
		randRead, keyringAdd, keyringSearch, keyringRead = origRand, origAdd, origSearch, origRead
	})
}

func TestSocketToken(t *testing.T) {
	if !keyring.Available() {
		t.Skip("kernel user keyring isn't usable for a round trip in this environment (e.g. no session-keyring link — common in CI/containers without a PAM login)")
	}

	tok := SocketToken()
	if tok == "" {
		t.Skip("user keyring unavailable")
	}
	assert.Len(t, tok, tokenByteLen*2, "token length")
	_, err := hex.DecodeString(tok)
	assert.NoError(t, err, "the token must be hex")
	assert.Equal(t, tok, SocketToken(), "the token must be stable within a login")
}

func TestReadSocketToken(t *testing.T) {
	if !keyring.Available() {
		t.Skip("kernel user keyring isn't usable for a round trip in this environment (e.g. no session-keyring link — common in CI/containers without a PAM login)")
	}

	// Reset to a known "no token yet" state, regardless of what earlier tests
	// in this process left behind, and restore it afterwards.
	var priorPayload []byte
	if s, ok := keyring.Search(tokenDescription); ok {
		priorPayload, _ = keyring.Read(s)
		_ = keyring.Unlink(s)
	}
	t.Cleanup(func() {
		if priorPayload != nil {
			_, _ = keyring.Add(tokenDescription, priorPayload)
		}
	})

	require.Empty(t, ReadSocketToken(), "ReadSocketToken must not create a token")
	_, ok := keyring.Search(tokenDescription)
	require.False(t, ok, "ReadSocketToken created a key in the @u keyring; it must only read")

	created := SocketToken()
	if created == "" {
		t.Skip("user keyring unavailable")
	}
	t.Cleanup(func() {
		if s, ok := keyring.Search(tokenDescription); ok {
			_ = keyring.Unlink(s)
		}
	})
	assert.Equal(t, created, ReadSocketToken(), "ReadSocketToken must return the token SocketToken created")
}

// TestReadTokenSeams covers readToken's branches without a live keyring by
// swapping the keyring seams: a search miss, a read failure, and a hit.
func TestReadTokenSeams(t *testing.T) {
	t.Run("search miss returns empty", func(t *testing.T) {
		saveTokenSeams(t)
		keyringSearch = func(string) (keyring.Serial, bool) { return 0, false }
		assert.Empty(t, ReadSocketToken(), "a search miss yields no token")
	})

	t.Run("read failure returns empty", func(t *testing.T) {
		saveTokenSeams(t)
		keyringSearch = func(string) (keyring.Serial, bool) { return 1, true }
		keyringRead = func(keyring.Serial) ([]byte, error) { return nil, errReadDenied }
		assert.Empty(t, ReadSocketToken(), "a read failure yields no token")
	})

	t.Run("hit returns the payload", func(t *testing.T) {
		saveTokenSeams(t)
		keyringSearch = func(string) (keyring.Serial, bool) { return 1, true }
		keyringRead = func(keyring.Serial) ([]byte, error) { return []byte("payload"), nil }
		assert.Equal(t, "payload", ReadSocketToken(), "a hit returns the stored payload")
	})
}

// TestSocketTokenSeams covers SocketToken's create path and its failure and
// convergence branches, all driven through the injected keyring/RNG seams.
func TestSocketTokenSeams(t *testing.T) {
	t.Run("returns an existing token without creating", func(t *testing.T) {
		saveTokenSeams(t)
		keyringSearch = func(string) (keyring.Serial, bool) { return 1, true }
		keyringRead = func(keyring.Serial) ([]byte, error) { return []byte("existing"), nil }
		keyringAdd = func(string, []byte) (keyring.Serial, error) {
			assert.Fail(t, "SocketToken created a key when one already existed")
			return 0, nil
		}
		assert.Equal(t, "existing", SocketToken(), "the existing token is returned as is")
	})

	t.Run("rand failure returns empty", func(t *testing.T) {
		saveTokenSeams(t)
		keyringSearch = func(string) (keyring.Serial, bool) { return 0, false }
		randRead = func([]byte) (int, error) { return 0, errNoEntropy }
		assert.Empty(t, SocketToken(), "no entropy yields no token")
	})

	t.Run("keyring add failure returns empty", func(t *testing.T) {
		saveTokenSeams(t)
		keyringSearch = func(string) (keyring.Serial, bool) { return 0, false }
		randRead = rand.Read
		keyringAdd = func(string, []byte) (keyring.Serial, error) { return 0, errAddDenied }
		assert.Empty(t, SocketToken(), "a refused add yields no token")
	})

	// F1: every shell of one login must land on the same socket, and the token
	// is what decides which. A keyring that takes the key and will not give it
	// back cannot be where the shells meet: what this process created, only this
	// process can name. Answering with it anyway hands each shell a token of its
	// own — a socket directory of its own, an agent of its own, and a passphrase
	// asked for again in every window.
	t.Run("a token that cannot be read back is not one a login can share", func(t *testing.T) {
		saveTokenSeams(t)
		keyringSearch = func(string) (keyring.Serial, bool) { return 0, false }
		randRead = rand.Read
		keyringAdd = func(string, []byte) (keyring.Serial, error) { return 1, nil }

		first, second := SocketToken(), SocketToken()

		assert.Equal(t, first, second,
			"two shells of one login must arrive at one answer, or they are two logins as far as the agent is concerned")
		assert.Empty(t, first,
			"and where there is nowhere to share one, the answer is the tokenless path rather than a private token")
	})

	t.Run("converges on the read-back value", func(t *testing.T) {
		saveTokenSeams(t)
		added := false
		keyringSearch = func(string) (keyring.Serial, bool) { return 1, added }
		keyringRead = func(keyring.Serial) ([]byte, error) { return []byte("winner"), nil }
		randRead = rand.Read
		keyringAdd = func(string, []byte) (keyring.Serial, error) { added = true; return 1, nil }
		assert.Equal(t, "winner", SocketToken(), "the read-back value wins")
	})
}
