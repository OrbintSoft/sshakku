//go:build linux

package paths

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/keyring"
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
	if len(tok) != tokenByteLen*2 {
		t.Errorf("token length = %d, want %d", len(tok), tokenByteLen*2)
	}
	if _, err := hex.DecodeString(tok); err != nil {
		t.Errorf("token is not hex: %v", err)
	}
	if again := SocketToken(); again != tok {
		t.Errorf("token not stable within a login: %q then %q", tok, again)
	}
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

	if tok := ReadSocketToken(); tok != "" {
		t.Fatalf("ReadSocketToken() = %q, want \"\" (must not create a token)", tok)
	}
	if _, ok := keyring.Search(tokenDescription); ok {
		t.Fatal("ReadSocketToken() created a key in the @u keyring; it must only read")
	}

	created := SocketToken()
	if created == "" {
		t.Skip("user keyring unavailable")
	}
	t.Cleanup(func() {
		if s, ok := keyring.Search(tokenDescription); ok {
			_ = keyring.Unlink(s)
		}
	})
	if got := ReadSocketToken(); got != created {
		t.Errorf("ReadSocketToken() = %q, want the token SocketToken created: %q", got, created)
	}
}

// TestReadTokenSeams covers readToken's branches without a live keyring by
// swapping the keyring seams: a search miss, a read failure, and a hit.
func TestReadTokenSeams(t *testing.T) {
	t.Run("search miss returns empty", func(t *testing.T) {
		saveTokenSeams(t)
		keyringSearch = func(string) (keyring.Serial, bool) { return 0, false }
		if tok := ReadSocketToken(); tok != "" {
			t.Errorf("ReadSocketToken() = %q, want \"\" on a search miss", tok)
		}
	})

	t.Run("read failure returns empty", func(t *testing.T) {
		saveTokenSeams(t)
		keyringSearch = func(string) (keyring.Serial, bool) { return 1, true }
		keyringRead = func(keyring.Serial) ([]byte, error) { return nil, errors.New("read denied") }
		if tok := ReadSocketToken(); tok != "" {
			t.Errorf("ReadSocketToken() = %q, want \"\" on a read failure", tok)
		}
	})

	t.Run("hit returns the payload", func(t *testing.T) {
		saveTokenSeams(t)
		keyringSearch = func(string) (keyring.Serial, bool) { return 1, true }
		keyringRead = func(keyring.Serial) ([]byte, error) { return []byte("payload"), nil }
		if tok := ReadSocketToken(); tok != "payload" {
			t.Errorf("ReadSocketToken() = %q, want payload", tok)
		}
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
			t.Error("SocketToken created a key when one already existed")
			return 0, nil
		}
		if tok := SocketToken(); tok != "existing" {
			t.Errorf("SocketToken() = %q, want existing", tok)
		}
	})

	t.Run("rand failure returns empty", func(t *testing.T) {
		saveTokenSeams(t)
		keyringSearch = func(string) (keyring.Serial, bool) { return 0, false }
		randRead = func([]byte) (int, error) { return 0, errors.New("no entropy") }
		if tok := SocketToken(); tok != "" {
			t.Errorf("SocketToken() = %q, want \"\" on rand failure", tok)
		}
	})

	t.Run("keyring add failure returns empty", func(t *testing.T) {
		saveTokenSeams(t)
		keyringSearch = func(string) (keyring.Serial, bool) { return 0, false }
		randRead = rand.Read
		keyringAdd = func(string, []byte) (keyring.Serial, error) { return 0, errors.New("add denied") }
		if tok := SocketToken(); tok != "" {
			t.Errorf("SocketToken() = %q, want \"\" on add failure", tok)
		}
	})

	t.Run("returns the freshly created token when read-back is empty", func(t *testing.T) {
		saveTokenSeams(t)
		keyringSearch = func(string) (keyring.Serial, bool) { return 0, false }
		randRead = rand.Read
		keyringAdd = func(string, []byte) (keyring.Serial, error) { return 1, nil }
		tok := SocketToken()
		if len(tok) != tokenByteLen*2 {
			t.Errorf("token length = %d, want %d", len(tok), tokenByteLen*2)
		}
		if _, err := hex.DecodeString(tok); err != nil {
			t.Errorf("token is not hex: %v", err)
		}
	})

	t.Run("converges on the read-back value", func(t *testing.T) {
		saveTokenSeams(t)
		added := false
		keyringSearch = func(string) (keyring.Serial, bool) { return 1, added }
		keyringRead = func(keyring.Serial) ([]byte, error) { return []byte("winner"), nil }
		randRead = rand.Read
		keyringAdd = func(string, []byte) (keyring.Serial, error) { added = true; return 1, nil }
		if tok := SocketToken(); tok != "winner" {
			t.Errorf("SocketToken() = %q, want winner (read-back convergence)", tok)
		}
	})
}
