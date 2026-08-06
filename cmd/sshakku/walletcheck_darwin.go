//go:build darwin

package main

import (
	"fmt"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/diagnose"
)

// platformWalletView describes the wallet this operating system provides
// itself: off Linux that is the OS keychain, reached through the system rather
// than through anything installed alongside, so there is no separate piece that
// could be missing and nothing to report as absent.
func (p walletProbe) platformWalletView(_ config.Settings, backend string) diagnose.WalletView {
	return diagnose.WalletView{Backend: backend}
}

// keepassxcSecretServiceRoute describes reaching KeePassXC over the Secret
// Service on a system that has none.
//
// The route is still described rather than silently swapped, because a route
// the user pinned is answered under its own name (F23) — but the answer here is
// not a missing piece to go and install. It is that this way in does not exist
// on this operating system, and another one has to be chosen.
func (p walletProbe) keepassxcSecretServiceRoute() []diagnose.Requirement {
	return []diagnose.Requirement{{
		Name: "secret service",
		Detail: fmt.Sprintf(
			"%s provides no freedesktop Secret Service — set keepassxc_route to native or cli", p.goos),
	}}
}

// realSecretServiceLook is the look a system with no session bus can take:
// there is nothing to ask and nothing that could answer.
func realSecretServiceLook(_, _ string) secretServiceLook {
	return secretServiceLook{}
}
