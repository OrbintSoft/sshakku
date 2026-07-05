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
