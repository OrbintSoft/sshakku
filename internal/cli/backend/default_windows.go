//go:build windows

package backend

import (
	"context"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys"
	"github.com/OrbintSoft/sshakku/internal/keys/wallet"
)

// newDefaultSecretBackend resolves the wallet used when the configuration
// names none: the store this system keeps itself, which is also the only one
// on offer here. user and log are unused — the store is scoped to the account
// the process is already running as, and there is nothing to fall back from —
// and are kept for signature parity with the other platforms.
//
// It is given the budget for something that answers by itself rather than the
// longer one kept for a wait on a person. This store raises no dialog and asks
// for no approval: what guards an entry is the account being signed in, which
// happened before this program started.
func newDefaultSecretBackend(_ context.Context, _ string, _ keys.Logger, settings config.Settings) (wallet.Backend, func()) {
	return &wallet.CredentialManager{
		ServicePrefix: settings.ServicePrefix,
		Timeout:       settings.CommandTimeout,
	}, func() {}
}
