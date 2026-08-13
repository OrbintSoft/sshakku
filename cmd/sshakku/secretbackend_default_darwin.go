//go:build darwin

package main

import (
	"context"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys"
)

// newDefaultSecretBackend resolves the default (secret-service) backend off
// Linux, where the freedesktop Secret Service D-Bus protocol has no equivalent.
// The native secret store is the OS keychain (the macOS keychain on Darwin), so
// the default maps to it rather than dialing a bus that isn't there. log is
// unused here — nothing to fall back from — but kept for signature parity with
// the Linux implementation.
//
// It is given the budget for a wait on a person, not the shorter one for
// something that answers by itself: the keychain usually replies at once, but it
// can put up its own approval dialog first, and from the outside those two are
// the same call. Waiting the shorter time means giving up while someone is still
// typing.
func newDefaultSecretBackend(_ context.Context, user string, log keys.Logger, settings config.Settings) (keys.SecretBackend, func()) {
	return newKeychainBackend(user, settings.InteractiveTimeout, settings.ServicePrefix), func() {}
}
