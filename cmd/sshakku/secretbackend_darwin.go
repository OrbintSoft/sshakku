//go:build darwin

package main

import (
	"time"

	"github.com/OrbintSoft/sshakku/internal/keys"
)

// newKeychainBackend returns a SecretBackend over the macOS keychain, scoped to
// user's items via the "account" attribute and bounded by timeout — the
// keychain is not run as a command, but it is still something waited on, and it
// can wait on an authorization of its own that never comes.
func newKeychainBackend(user string, timeout time.Duration) keys.SecretBackend {
	return &keys.KeychainBackend{Client: keys.DarwinKeychainClient{}, Account: user, Timeout: timeout}
}
