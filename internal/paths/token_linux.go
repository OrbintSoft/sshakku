//go:build linux

package paths

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/OrbintSoft/sshakku/internal/keyring"
)

const (
	tokenDescription = app + "-socket-token"
	tokenByteLen     = 16
)

// SocketToken returns the per-login token shared via the @u user keyring,
// creating it on first use. Every shell of a login converges on a single value:
// the keyring is keyed by description, so a racing creator only updates the same
// key, and we read the canonical payload back. It returns "" (no error) when the
// keyring is unavailable, so the caller degrades to a tokenless path.
func SocketToken() string {
	if tok := readToken(); tok != "" {
		return tok
	}
	b := make([]byte, tokenByteLen)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	token := hex.EncodeToString(b)
	if _, err := keyring.Add(tokenDescription, []byte(token)); err != nil {
		return ""
	}
	// Read back so all racing creators converge on whichever payload won.
	if back := readToken(); back != "" {
		return back
	}
	return token
}

// ReadSocketToken returns the per-login token from the @u user keyring without
// creating one, unlike SocketToken. It reads the keyring of the calling
// process's own effective uid — never another user's — so a caller that needs
// another uid's token must first assume that uid's identity (e.g. by spawning a
// child process with that uid's credentials) before calling this. Returns ""
// when no token exists yet or the keyring is unavailable.
func ReadSocketToken() string {
	return readToken()
}

// readToken returns the socket-token payload from the @u keyring, or "" if it is
// absent or unreadable.
func readToken() string {
	s, ok := keyring.Search(tokenDescription)
	if !ok {
		return ""
	}
	payload, err := keyring.Read(s)
	if err != nil {
		return ""
	}
	return string(payload)
}
