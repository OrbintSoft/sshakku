//go:build linux

package main

import (
	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/diagnose"
)

// platformWalletView describes the wallet this operating system provides
// itself: on Linux the freedesktop Secret Service, which is reached over the
// session bus and so has a piece that can be absent.
//
// Any other name reaching here has already been resolved by the configuration
// layer, which only ever yields a wallet this system has.
func (p walletProbe) platformWalletView(backend string) diagnose.WalletView {
	if backend == config.SecretBackendSecretService {
		return diagnose.WalletView{
			Backend:      backend,
			Requirements: []diagnose.Requirement{p.sessionBus()},
		}
	}
	return diagnose.WalletView{Backend: backend}
}

// keepassxcSecretServiceRoute describes reaching KeePassXC over the Secret
// Service, which on Linux means over the session bus like any other wallet
// behind that API.
func (p walletProbe) keepassxcSecretServiceRoute() diagnose.Requirement {
	return p.sessionBus()
}

// sessionBus reports whether there is a D-Bus session bus to reach a Secret
// Service over. Only its address is checked: asking the bus who owns the
// service would be a conversation with the wallet, and this report does not
// have those.
func (p walletProbe) sessionBus() diagnose.Requirement {
	const name = "session bus"
	if p.busAddress != "" {
		return diagnose.Requirement{Name: name, Detail: p.busAddress, Present: true}
	}
	return diagnose.Requirement{
		Name:   name,
		Detail: "DBUS_SESSION_BUS_ADDRESS is unset; this shell is not in a desktop session",
	}
}
