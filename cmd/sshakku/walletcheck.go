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
	// goos is the platform, so the answer can be checked from any of them.
	goos string
}

// realWalletProbe looks at the machine this is running on.
func realWalletProbe() walletProbe {
	return walletProbe{
		onPath:     exec.LookPath,
		exists:     func(path string) bool { _, err := os.Stat(path); return err == nil },
		listening:  keepassxc.SocketPaths(),
		busAddress: os.Getenv("DBUS_SESSION_BUS_ADDRESS"),
		goos:       runtime.GOOS,
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
		return probe.platformWalletView(backend)
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
		view.Requirements = append(view.Requirements, p.keepassxcSecretServiceRoute())
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
