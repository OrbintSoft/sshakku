//go:build linux

package paths

import (
	"encoding/hex"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/keyring"
)

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
