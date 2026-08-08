package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/diagnose"
	"github.com/OrbintSoft/sshakku/internal/keepassxc"
)

// walletProbe is how the checks below find out whether something is here. Both
// only look: nothing they do stores, reads or removes a passphrase, which is
// what keeps the doctor's plain report safe to run at any time. Proving a
// wallet actually works is a separate, deliberate act — `--test-backend`.
type walletProbe struct {
	// onPath reports where an executable is, or an error when it is nowhere.
	onPath func(name string) (string, error)
	// exists reports whether a path is there, whatever it is.
	exists func(path string) bool
	// listening are the addresses a running KeePassXC would be reachable at.
	listening []string
	// busAddress is DBUS_SESSION_BUS_ADDRESS as this shell sees it.
	busAddress string
	// hasScreen is whether this session has a display of any kind, which is
	// what creating a compartment in some wallets takes.
	hasScreen bool
	// look asks the session bus, and a wallet already answering on it, about
	// themselves. Nil where this system has no such bus.
	look func(alias, label string) secretServiceLook
	// goos is the platform, so the answer can be checked from any of them.
	goos string
}

// secretServiceLook is what looking at the session bus found, as plain data:
// which of the two ways to a wallet is open, and whether the compartment
// SSHakku would keep its entries in has already been made. It names no D-Bus
// type, so what the report makes of each combination stays checkable from a
// machine that has no Secret Service at all.
type secretServiceLook struct {
	// running is whether a wallet answers on the bus now.
	running bool
	// activatable is whether one would be started when first needed.
	activatable bool
	// collectionFound is whether the compartment is already there; it says
	// nothing unless running.
	collectionFound bool
	// askFailed is whether a wallet that was there to ask did not answer, so
	// "not there" is never confused with "could not be told".
	askFailed bool
	// lookFailed is whether the bus itself could not be asked, in which case
	// nothing below it was established either.
	lookFailed bool
}

// realWalletView describes the configured wallet as this machine actually is.
func realWalletView(settings config.Settings) diagnose.WalletView {
	return walletView(settings, realWalletProbe())
}

// realWalletProbe looks at the machine this is running on.
func realWalletProbe() walletProbe {
	return walletProbe{
		onPath:     exec.LookPath,
		exists:     func(path string) bool { _, err := os.Stat(path); return err == nil },
		listening:  keepassxc.SocketPaths(),
		busAddress: os.Getenv("DBUS_SESSION_BUS_ADDRESS"),
		hasScreen:  os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "",
		look:       realSecretServiceLook,
		goos:       runtime.GOOS,
	}
}

// serviceRequirement states whether a wallet is reachable on the bus. A wallet
// that is not running but would be started when first needed is not a missing
// piece — nothing has asked for it yet.
func serviceRequirement(look secretServiceLook) diagnose.Requirement {
	const name = "secret service"
	switch {
	case look.lookFailed:
		return diagnose.Requirement{
			Name:         name,
			Detail:       "the session bus could not be asked what is on it",
			Undetermined: true,
		}
	case look.running:
		return diagnose.Requirement{Name: name, Detail: "a wallet is answering on this bus", Present: true}
	case look.activatable:
		return diagnose.Requirement{
			Name:    name,
			Detail:  "not running; the bus starts one when a passphrase is first needed",
			Present: true,
		}
	default:
		return diagnose.Requirement{
			Name: name,
			Detail: "nothing answers to org.freedesktop.secrets on this bus, and nothing here would start one; " +
				"start your desktop's keyring, or configure a wallet that does not need one",
		}
	}
}

// compartmentRequirement states whether the compartment SSHakku would keep its
// entries in is there, and when it is not, whether this session could make one.
//
// Not being there is only a problem where it cannot be created: with a screen
// the first passphrase saved makes it through a dialog, and until then there is
// nothing wrong to report. Without one there is no dialog to answer, which is
// the whole of what goes wrong on a machine reached over ssh.
func compartmentRequirement(compartment string, look secretServiceLook, hasScreen bool) diagnose.Requirement {
	const name = "compartment"
	switch {
	case look.lookFailed || !look.running:
		return diagnose.Requirement{
			Name:         name,
			Detail:       "no wallet was answering to ask; sshakku doctor --test-backend starts one and proves the round trip",
			Undetermined: true,
		}
	case look.askFailed:
		return diagnose.Requirement{
			Name:         name,
			Detail:       "the wallet did not answer when asked whether it holds one",
			Undetermined: true,
		}
	case look.collectionFound:
		return diagnose.Requirement{Name: name, Detail: compartment, Present: true}
	case hasScreen:
		return diagnose.Requirement{
			Name:    name,
			Detail:  fmt.Sprintf("%q is not there yet; the first passphrase saved creates it, through a dialog on this screen", compartment),
			Present: true,
		}
	default:
		return diagnose.Requirement{
			Name: name,
			Detail: fmt.Sprintf(
				"%q is not there and this session has no screen to create it on — passphrases cannot be saved here and "+
					"you are asked for each one; create it once from a desktop session and it works here from then on",
				compartment),
		}
	}
}

// walletView describes the wallet settings selects and what it needs, for the
// doctor to present.
//
// The name is taken as it comes: the configuration layer has already replaced
// anything this system cannot offer with the wallet it will actually open, so
// what arrives here is what the user's passphrases go into. Naming something
// else would be describing a wallet nobody is using.
//
// The wallets that are the operating system's own are described by
// platformWalletView, which is compiled per platform — the freedesktop pieces
// exist on Linux alone, and there is nothing to say about them elsewhere.
func walletView(settings config.Settings, probe walletProbe) diagnose.WalletView {
	switch backend := settings.SecretBackend; backend {
	case config.SecretBackendKeePassXC:
		return probe.keepassxcView(settings)
	case config.SecretBackendOnePassword:
		return diagnose.WalletView{
			Backend:      backend,
			Requirements: []diagnose.Requirement{probe.tool("op", "1Password's command-line tool")},
		}
	case config.SecretBackendBitwarden:
		return diagnose.WalletView{
			Backend:      backend,
			Requirements: []diagnose.Requirement{probe.tool("bw", "Bitwarden's command-line tool")},
		}
	default:
		return probe.platformWalletView(settings, backend)
	}
}

// keepassxcView describes KeePassXC by the route that would actually be taken,
// since the three routes need entirely different things present.
func (p walletProbe) keepassxcView(settings config.Settings) diagnose.WalletView {
	route := settings.KeePassXCRoute
	if route == "" || route == config.KeePassXCRouteAuto {
		route = keepassxcRouteFor(p.goos)
	}
	view := diagnose.WalletView{Backend: config.SecretBackendKeePassXC, Route: route}

	switch route {
	case config.KeePassXCRouteCLI:
		view.Requirements = append(view.Requirements,
			p.tool("keepassxc-cli", "KeePassXC's command-line tool"),
			p.databaseFile(settings.KeePassXCDatabase))
	case config.KeePassXCRouteNative:
		view.Requirements = append(view.Requirements, p.keepassxcListening())
	default:
		// The Secret Service route is the one that cannot exist everywhere.
		// Unlike the wallet name, a route the user pinned is reported under its
		// own name rather than swapped (F23), so this route is still described
		// off Linux — but what there is to say about it differs so completely
		// that each platform says its own.
		view.Requirements = append(view.Requirements, p.keepassxcSecretServiceRoute()...)
	}
	return view
}

// tool reports whether an executable this wallet drives is on PATH.
func (p walletProbe) tool(name, description string) diagnose.Requirement {
	path, err := p.onPath(name)
	if err != nil {
		return diagnose.Requirement{
			Name:   name,
			Detail: fmt.Sprintf("not on PATH; install %s, or configure a wallet that does not need it", description),
		}
	}
	return diagnose.Requirement{Name: name, Detail: path, Present: true}
}

// databaseFile reports whether the database the cli route works on is there.
// The route cannot discover one: a file on disk does not announce itself.
func (p walletProbe) databaseFile(path string) diagnose.Requirement {
	const name = "database"
	if path == "" {
		return diagnose.Requirement{Name: name, Detail: "keepassxc_database has to name the database file to use"}
	}
	if !p.exists(path) {
		return diagnose.Requirement{Name: name, Detail: fmt.Sprintf("%s does not exist", path)}
	}
	return diagnose.Requirement{Name: name, Detail: path, Present: true}
}

// keepassxcListening reports whether a KeePassXC is reachable over its local
// protocol, which it is only while running with browser integration enabled.
func (p walletProbe) keepassxcListening() diagnose.Requirement {
	const name = "KeePassXC"
	for _, address := range p.listening {
		if p.exists(address) {
			return diagnose.Requirement{Name: name, Detail: address, Present: true}
		}
	}
	return diagnose.Requirement{
		Name:   name,
		Detail: "nothing is listening; start KeePassXC, or enable browser integration in its settings",
	}
}
