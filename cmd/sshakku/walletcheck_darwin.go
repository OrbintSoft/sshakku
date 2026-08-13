//go:build darwin

package main

import (
	"context"
	"fmt"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/diagnose"
)

// platformWalletView describes the wallet this operating system provides
// itself: off Linux that is the OS keychain, reached through the system rather
// than through anything installed alongside, so there is no separate piece that
// could be missing and nothing to report as absent.
func (p walletProbe) platformWalletView(_ context.Context, _ config.Settings, backend string) diagnose.WalletView {
	return diagnose.WalletView{Backend: backend}
}

// keepassxcSecretServiceRoute describes reaching KeePassXC over the Secret
// Service on a system that has none.
//
// The route is still described rather than silently swapped, because a route
// the user pinned is answered under its own name (F23) — but the answer here is
// not a missing piece to go and install. It is that this way in does not exist
// on this operating system, and another one has to be chosen.
func (p walletProbe) keepassxcSecretServiceRoute(ctx context.Context) []diagnose.Requirement {
	return []diagnose.Requirement{{
		Name: "secret service",
		Detail: fmt.Sprintf(
			"%s provides no freedesktop Secret Service — set keepassxc_route to native or cli", p.goos),
	}}
}

// realMakeCompartment is how this system makes the compartment its wallet keeps
// SSHakku's entries in, and the Keychain has none to make: entries go into the
// login keychain the account already has, so there is nothing to create and
// nothing that could be missing. Left nil rather than given a body that does
// nothing, because doing nothing successfully is not the same as having nothing
// to do.
var realMakeCompartment func(ctx context.Context, settings config.Settings) (string, error)

// realSecretServiceLook is the look this system can take at a session bus, and
// macOS has none: no bus, nothing on it, and nothing that could answer. It is
// left nil rather than given a body that returns emptiness, because there is no
// question here to answer at all.
var realSecretServiceLook func(ctx context.Context, alias, label string) secretServiceLook
