//go:build windows

package walletcheck

import (
	"context"
	"fmt"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/diagnose"
)

// platformWalletView describes the wallet this operating system provides
// itself: the Credential Manager, which needs nothing installed, nothing
// running and nothing configured, so there is no piece to report as missing.
//
// What it does report is what guards an entry, which is the one thing about
// this wallet a reader would otherwise get wrong. It never asks for anything,
// and a wallet that never asks looks from the outside exactly like one that
// has already been unlocked — so the report says, in the section describing
// the wallet, that there was nothing to unlock in the first place.
func (p walletProbe) platformWalletView(_ context.Context, _ config.Settings, backend string) diagnose.WalletView {
	return diagnose.WalletView{Backend: backend, Guard: credentialStoreGuard}
}

// credentialStoreGuard is what stands between a stored passphrase and whoever
// wants to read it here: the account being signed in, and nothing further. The
// entry is encrypted under this account and handed back to anything running as
// it, with no prompt, no per-entry unlock and no permission asked per program.
const credentialStoreGuard = "this account's own sign-in, and nothing beyond it — " +
	"there is no separate password and nothing to unlock, so any program running as you can read what is stored here"

// keepassxcSecretServiceRoute describes reaching KeePassXC over the Secret
// Service on a system that has none.
//
// The route is still described rather than silently swapped, because a route
// the user pinned is answered under its own name — but the answer here is not a
// missing piece to go and install. It is that this way in does not exist on
// this operating system, and another one has to be chosen.
func (p walletProbe) keepassxcSecretServiceRoute(ctx context.Context) []diagnose.Requirement {
	return []diagnose.Requirement{{
		Name: "secret service",
		Detail: fmt.Sprintf(
			"%s provides no freedesktop Secret Service — set keepassxc_route to native or cli", p.goos),
	}}
}

// MakeCompartment is how this system makes the compartment its wallet keeps
// SSHakku's entries in. There is no wallet here to make one in, so it is left
// nil rather than given a body that does nothing: doing nothing successfully is
// not the same as having nothing to do, and doctor tells those apart.
var MakeCompartment func(ctx context.Context, settings config.Settings) (string, error)

// realSecretServiceLook is the look this system can take at a session bus, and
// Windows has none: no bus, nothing on it, and nothing that could answer. It is
// left nil rather than given a body that returns emptiness, because there is no
// question here to answer at all.
var realSecretServiceLook func(ctx context.Context, alias, label string) SecretServiceLook
