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

// defaultSecretBackendName is the wallet used when the user named none: the
// freedesktop Secret Service on Linux, the OS keychain everywhere else — the
// same choice newDefaultSecretBackend makes, named rather than built. Taking
// the platform as an argument keeps it checkable from any of them.
func defaultSecretBackendName(goos string) string {
	if goos == "linux" {
		return config.SecretBackendSecretService
	}
	return config.SecretBackendKeychain
}

// walletView describes the wallet settings selects and what it needs, for the
// doctor to present. An unknown backend name yields a view naming it and
// nothing else: saying "this is what you configured" is still the answer to
// "which wallet would be used", and the config layer is where a bad name is
// rejected.
func walletView(settings config.Settings, probe walletProbe) diagnose.WalletView {
	backend := settings.SecretBackend
	if backend == "" {
		backend = defaultSecretBackendName(probe.goos)
	}

	switch backend {
	case config.SecretBackendKeePassXC:
		return probe.keepassxcView(settings)
	case config.SecretBackendSecretService:
		return diagnose.WalletView{
			Backend:      backend,
			Requirements: []diagnose.Requirement{probe.sessionBus()},
		}
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
		// The Keychain, and anything else that reaches its wallet through the
		// operating system itself: there is no separate piece to be missing.
		return diagnose.WalletView{Backend: backend}
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
		// The Secret Service route is the one that cannot exist everywhere:
		// off Linux there is no such API to reach KeePassXC through, and the
		// answer is to pin a route that does work rather than to install
		// anything.
		if p.goos != "linux" {
			view.Requirements = append(view.Requirements, diagnose.Requirement{
				Name: "secret service",
				Detail: fmt.Sprintf(
					"%s provides no freedesktop Secret Service — set keepassxc_route to native or cli", p.goos),
			})
			return view
		}
		view.Requirements = append(view.Requirements, p.sessionBus())
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
