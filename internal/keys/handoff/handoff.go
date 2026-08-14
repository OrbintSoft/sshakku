// Package handoff carries one passphrase from the process that loaded a key to
// the SSH_ASKPASS helper ssh-add runs to ask for it, without the passphrase
// passing through any of the places it must never appear.
//
// ssh-add takes a passphrase only from its askpass helper, and that helper is a
// separate process, so the passphrase has to cross a process boundary. What
// crosses it here is a token — a handle, worth nothing on its own — travelling
// in the environment; the passphrase itself stays in the kernel, in a keyring
// entry on Linux or a socket buffer on Darwin, never in argv, an environment
// variable, or a file. Stash puts it there and Fetch takes it out, once: the
// token is invalidated on the first redemption whether or not the read
// succeeded, and expires on its own if nobody ever comes for it.
package handoff

import (
	"crypto/rand"
	"encoding/hex"
)

// tokenBytes sizes the random one-shot token: a kernel-keyring description on
// Linux, a socket filename on Darwin. Kept short because on Darwin the token
// becomes part of an AF_UNIX socket path, which the kernel caps at 104 bytes
// (108 on Linux) — 8 bytes (16 hex chars) still leaves concurrent key loads no
// realistic chance of colliding.
const tokenBytes = 8

// randRead is the token RNG, a seam so randomToken's read-failure branch
// (which crypto/rand practically never takes) can be exercised.
var randRead = rand.Read

// randomToken returns a unique random hex string, so concurrent key loads never
// collide on (or cross-read) one another's stashed passphrase.
func randomToken() (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := randRead(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// EnvToken names the environment variable the token travels to the askpass
// helper in. Only the token — a handle — crosses the environment; the
// passphrase itself never does.
const EnvToken = "SSHAKKU_HANDOFF_TOKEN"
