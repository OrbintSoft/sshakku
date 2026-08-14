//go:build linux

package walletcheck

import (
	"context"
	"time"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/diagnose"
	"github.com/OrbintSoft/sshakku/internal/keys/wallet"
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
func (p walletProbe) platformWalletView(ctx context.Context, settings config.Settings, backend string) diagnose.WalletView {
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
	alias, label := wallet.SecretServiceCollectionNames(settings.SecretContainer)
	look := p.look(ctx, alias, label)
	view.Requirements = append(view.Requirements,
		serviceRequirement(look),
		CompartmentRequirement(label, look, p.hasScreen))
	return view
}

// keepassxcSecretServiceRoute describes reaching KeePassXC over the Secret
// Service, which on Linux means over the session bus, with KeePassXC itself as
// the wallet answering there. No compartment is described: the group this route
// keeps entries in is KeePassXC's to make inside a database the user opened, not
// something the desktop is asked to create.
func (p walletProbe) keepassxcSecretServiceRoute(ctx context.Context) []diagnose.Requirement {
	bus := p.sessionBus()
	if !bus.Present || p.look == nil {
		return []diagnose.Requirement{bus}
	}
	return []diagnose.Requirement{bus, serviceRequirement(p.look(ctx, "", ""))}
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

// MakeCompartment makes the compartment entries would be stored in, under
// the same names the report asked about, and returns the name it made. The
// wallet decides what making one takes — GNOME Keyring asks for a password to
// protect the new compartment with — so this is the one place in the doctor that
// may raise a dialog, and it is reached only from --fix.
//
// A compartment that is already there is returned as it is, which is why the
// caller decides whether there was anything to make: what this reports is what
// it resolved, not what it created.
func MakeCompartment(ctx context.Context, settings config.Settings) (string, error) {
	alias, label := wallet.SecretServiceCollectionNames(settings.SecretContainer)

	client, err := secretservice.NewClient(ctx)
	if err != nil {
		return "", err
	}
	// Everything below acts against a wallet that is really answering, which a
	// unit test has no stand-in for; the failure to reach a bus at all, above,
	// stays unit-tested, and the act itself is driven through the real binary
	// by gnome-keyring-fix-compartment-scenario.sh. Each straight-line block
	// carries its own ignore marker because go-ignore-cov scopes per block.
	//coverage:ignore
	defer func() {
		//coverage:ignore
		_ = client.Close(ctx)
	}()

	//coverage:ignore
	if _, err := client.Collection(ctx, alias, label); err != nil {
		//coverage:ignore
		return "", err
	}
	//coverage:ignore
	return label, nil
}

// realSecretServiceLook asks the bus, and a wallet already answering on it,
// about themselves. A look that fails is reported as a wallet that could not be
// asked, never as one that is not there: the report says what it does not know.
func realSecretServiceLook(ctx context.Context, alias, label string) SecretServiceLook {
	look, err := secretservice.LookForCollection(ctx, alias, label, lookTimeout)
	if err != nil {
		return SecretServiceLook{LookFailed: true}
	}
	return SecretServiceLook{
		Running:         look.Running,
		Activatable:     look.Activatable,
		CollectionFound: look.CollectionFound,
		AskFailed:       look.AskErr != nil,
	}
}
