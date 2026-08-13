//go:build windows

package main

import (
	"context"
	"errors"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys"
)

// errNoWallet is what the default backend reports here. This system's own
// wallet is the Credential Manager, which sshakku cannot yet read or write, and
// no other wallet is on offer either — so an unnamed backend has nothing to
// open.
var errNoWallet = errors.New("no secret backend is available on windows")

// newDefaultSecretBackend resolves the wallet used when the configuration names
// none. It reports the reason on every operation rather than behaving like an
// empty wallet: a miss would send the loader off to ask for a passphrase with
// no explanation, and would let a later store believe it had saved one.
func newDefaultSecretBackend(context.Context, string, keys.Logger, config.Settings) (keys.SecretBackend, func()) {
	return keys.UnavailableBackend{Reason: errNoWallet}, func() {}
}
