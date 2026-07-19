//go:build !darwin

package main

import "github.com/OrbintSoft/sshakku/internal/keys"

// newKeychainBackend returns a SecretBackend that reports
// keys.ErrKeychainUnavailable on every call: the macOS keychain doesn't
// exist off Darwin, so selecting secret_backend = "keychain" here is a
// configuration mistake, not something to silently degrade.
func newKeychainBackend(user string) keys.SecretBackend {
	return &keys.KeychainBackend{Client: keys.NoKeychainClient{}, Account: user}
}
