package main

import (
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys"
	"github.com/OrbintSoft/sshakku/internal/paths"
)

// newKeePassXCBackend opens KeePassXC by whichever route settings names.
//
// The user names the wallet with secret_backend; keepassxc_route says how to
// reach it, and a named route is used and no other. Only "auto" chooses, and
// only "auto" may fall back: a route the user pinned that cannot work is
// reported as unavailable under its own name, because silently using a
// different one would answer a question the user did not ask.
func newKeePassXCBackend(user string, log keys.Logger, settings config.Settings) (keys.SecretBackend, func()) {
	route := settings.KeePassXCRoute
	if route == "" || route == config.KeePassXCRouteAuto {
		route = keepassxcRouteFor(runtime.GOOS)
	}
	if reason := keepassxcRouteUnavailable(route, runtime.GOOS, settings.KeePassXCDatabase); reason != nil {
		return keys.UnavailableBackend{Reason: reason}, func() {}
	}
	switch route {
	case config.KeePassXCRouteNative:
		return newKeePassXCNativeRoute(settings), func() {}
	case config.KeePassXCRouteCLI:
		return newKeePassXCCLIRoute(settings), func() {}
	default:
		// The Secret Service route: KeePassXC implements that API itself, so
		// reaching it is the same as reaching any other wallet behind it and
		// there is nothing KeePassXC-specific to do.
		return newDefaultSecretBackend(user, log, settings)
	}
}

// keepassxcRouteFor is the route SSHakku picks when the user named none. Taking
// the platform as an argument keeps the choice checkable from any platform,
// rather than only from the one making it.
//
// Linux gets the Secret Service because KeePassXC implements that API itself
// and it is the path Linux users are already on; everywhere else gets the local
// protocol, which has the same preconditions and so keeps a refill silent.
func keepassxcRouteFor(goos string) string {
	if goos == "linux" {
		return config.KeePassXCRouteSecretService
	}
	return config.KeePassXCRouteNative
}

// keepassxcRouteUnavailable reports why a route cannot work here, or nil if it
// can. Routes are not tied to a platform — only the defaults are — so the only
// platform-bound one is the Secret Service, a freedesktop API no macOS system
// provides. The CLI route is bound to something else: a database file, which it
// cannot discover, because a file on disk does not announce itself the way a
// running KeePassXC knows what it has open.
func keepassxcRouteUnavailable(route, goos, database string) error {
	switch route {
	case config.KeePassXCRouteSecretService:
		if goos != "linux" {
			return fmt.Errorf("the secret-service route needs the freedesktop Secret Service, which %s does not provide", goos)
		}
		return nil
	case config.KeePassXCRouteCLI:
		if database == "" {
			return fmt.Errorf("the cli route works on a database file, so keepassxc_database has to name one")
		}
		return nil
	default:
		return nil
	}
}

// newKeePassXCCLIRoute runs keepassxc-cli against the database file, with no
// KeePassXC running. It asks for the database password, so it is bounded by the
// interactive budget — the one for a command waiting on a person — rather than
// the shorter budget for a command expected to answer on its own.
func newKeePassXCCLIRoute(settings config.Settings) keys.SecretBackend {
	return &keys.KeePassXCCLIBackend{
		Runner:   keys.ExecRunner{Timeout: settings.CommandTimeout},
		Prompter: newWalletPasswordPrompter(settings),
		Database: settings.KeePassXCDatabase,
		KeyFile:  settings.KeePassXCKeyFile,
		Timeout:  settings.InteractiveTimeout,
	}
}

// newKeePassXCNativeRoute speaks KeePassXC's local protocol to a running,
// unlocked instance.
//
// Two budgets, because there are two kinds of wait: KeePassXC answers a lookup
// by itself and quickly, while the one-time approval it raises is answered by
// the user, at their own pace.
func newKeePassXCNativeRoute(settings config.Settings) keys.SecretBackend {
	return keys.KeePassXCBackend{
		Associations:       keys.FileAssociationStore{Path: keepassxcAssociationPath()},
		Timeout:            settings.CommandTimeout,
		InteractiveTimeout: settings.InteractiveTimeout,
	}
}

// keepassxcAssociationPath is where the approval KeePassXC granted this client
// is kept, so the user approves it once rather than on every run.
func keepassxcAssociationPath() string {
	return filepath.Join(paths.Resolve(paths.FromOS(), paths.ProbeDir).StateDir, "keepassxc-association.json")
}
