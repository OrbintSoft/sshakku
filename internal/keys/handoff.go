package keys

import (
	"crypto/rand"
	"encoding/hex"
)

// handoffTokenBytes sizes the random one-shot passphrase-handoff token: a
// kernel-keyring description on Linux, a socket filename on Darwin (see
// handoff_linux.go, handoff_darwin.go, handoff_socket.go).
const handoffTokenBytes = 16

// randomHandoffToken returns a unique random hex string, so concurrent key
// loads never collide on (or cross-read) one another's stashed passphrase.
func randomHandoffToken() (string, error) {
	b := make([]byte, handoffTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// FetchHandoff redeems token for the one-shot passphrase AddWithAskpass
// stashed, invalidating it whether or not the read succeeds. Called by the
// SSH_ASKPASS child process, never by the loader itself.
func FetchHandoff(token string) (string, error) {
	return fetchPassphrase(token)
}
