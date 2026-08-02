//go:build darwin

package main

import (
	"time"

	"github.com/OrbintSoft/sshakku/internal/keys"
)

// newKeychainBackend returns a SecretBackend over the macOS keychain, scoped to
// user's items via the "account" attribute and bounded by timeout — the
// keychain is not run as a command, but it is still something waited on, and it
// can wait on an authorization of its own that never comes. prefix is the name
// sshakku's own items carry there, the login keychain being shared with every
// other program that keeps a password.
func newKeychainBackend(user string, timeout time.Duration, prefix string) keys.SecretBackend {
	return &keys.KeychainBackend{
		Client:        keys.DarwinKeychainClient{},
		Account:       user,
		Timeout:       timeout,
		ServicePrefix: prefix,
	}
}
