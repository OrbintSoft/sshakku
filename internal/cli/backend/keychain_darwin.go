//go:build darwin

package backend

import (
	"time"

	"github.com/OrbintSoft/sshakku/internal/keys/wallet"
)

// newKeychain returns a wallet.Backend over the macOS keychain, scoped to
// user's items via the "account" attribute and bounded by timeout — the
// keychain is not run as a command, but it is still something waited on, and it
// can wait on an authorization of its own that never comes. prefix is the name
// sshakku's own items carry there, the login keychain being shared with every
// other program that keeps a password.
func newKeychain(user string, timeout time.Duration, prefix string) wallet.Backend {
	return &wallet.Keychain{
		Client:        wallet.DarwinKeychainClient{},
		Account:       user,
		Timeout:       timeout,
		ServicePrefix: prefix,
	}
}
