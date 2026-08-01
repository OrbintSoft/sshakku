//go:build darwin

package main

import (
	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys"
)

// newDefaultSecretBackend resolves the default (secret-service) backend off
// Linux, where the freedesktop Secret Service D-Bus protocol has no equivalent.
// The native secret store is the OS keychain (the macOS keychain on Darwin), so
// the default maps to it rather than dialing a bus that isn't there. log is
// unused here — nothing to fall back from — but kept for signature parity with
// the Linux implementation, as are the command budgets in settings: the
// keychain is reached in-process, not by running anything.
func newDefaultSecretBackend(user string, log keys.Logger, settings config.Settings) (keys.SecretBackend, func()) {
	return newKeychainBackend(user), func() {}
}
