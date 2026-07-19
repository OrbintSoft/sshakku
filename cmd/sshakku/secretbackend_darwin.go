//go:build darwin

package main

import "github.com/OrbintSoft/sshakku/internal/keys"

// newKeychainBackend returns a SecretBackend over the macOS keychain,
// scoped to user's items via the "account" attribute.
func newKeychainBackend(user string) keys.SecretBackend {
	return &keys.KeychainBackend{Client: keys.DarwinKeychainClient{}, Account: user}
}
