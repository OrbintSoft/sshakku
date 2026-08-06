//go:build linux

package main

import (
	"time"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/diagnose"
	"github.com/OrbintSoft/sshakku/internal/keys"
	"github.com/OrbintSoft/sshakku/internal/secretservice"
)

// lookTimeout bounds each question the report asks the session bus or a wallet
// on it. A look is over in a moment or it is not worth having: a wallet that has
// stopped answering must cost the report a pause, never the report itself.
const lookTimeout = 2 * time.Second

// platformWalletView describes the wallet this operating system provides
// itself: on Linux the freedesktop Secret Service, which is reached over the
// session bus, needs something answering there, and keeps SSHakku's entries in
// a compartment that has to exist.
//
// Any other name reaching here has already been resolved by the configuration
// layer, which only ever yields a wallet this system has.
func (p walletProbe) platformWalletView(settings config.Settings, backend string) diagnose.WalletView {
	if backend != config.SecretBackendSecretService {
		return diagnose.WalletView{Backend: backend}
	}

	view := diagnose.WalletView{Backend: backend}
	bus := p.sessionBus()
	view.Requirements = append(view.Requirements, bus)
	if !bus.Present || p.look == nil {
		return view
	}

	// The compartment is asked about under the names entries would be stored
	// under, so what the report describes is the one that would be used.
	alias, label := keys.SecretServiceCollectionNames(settings.SecretContainer)
	look := p.look(alias, label)
	view.Requirements = append(view.Requirements,
		serviceRequirement(look),
		compartmentRequirement(label, look, p.hasScreen))
	return view
}

// keepassxcSecretServiceRoute describes reaching KeePassXC over the Secret
// Service, which on Linux means over the session bus, with KeePassXC itself as
// the wallet answering there. No compartment is described: the group this route
// keeps entries in is KeePassXC's to make inside a database the user opened, not
// something the desktop is asked to create.
func (p walletProbe) keepassxcSecretServiceRoute() []diagnose.Requirement {
	bus := p.sessionBus()
	if !bus.Present || p.look == nil {
		return []diagnose.Requirement{bus}
	}
	return []diagnose.Requirement{bus, serviceRequirement(p.look("", ""))}
}

// sessionBus reports whether there is a D-Bus session bus to reach a Secret
// Service over. Only its address is checked here: what is on the bus is a
// separate question, asked separately.
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

// realSecretServiceLook asks the bus, and a wallet already answering on it,
// about themselves. A look that fails is reported as a wallet that could not be
// asked, never as one that is not there: the report says what it does not know.
func realSecretServiceLook(alias, label string) secretServiceLook {
	look, err := secretservice.LookForCollection(alias, label, lookTimeout)
	if err != nil {
		return secretServiceLook{lookFailed: true}
	}
	return secretServiceLook{
		running:         look.Running,
		activatable:     look.Activatable,
		collectionFound: look.CollectionFound,
		askFailed:       look.AskErr != nil,
	}
}
