package main

import (
	"os"
	"runtime"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys"
)

// newSecretBackend opens the secret backend settings.SecretBackend selects
// (config.toml's secret_backend key). 1password and bitwarden shell out to
// their own CLI; the wallet the operating system provides itself is opened by
// newDefaultSecretBackend (the freedesktop Secret Service on Linux, the
// keychain off it), which is also what an unnamed backend gets. The returned
// func releases any resource the backend opened and must always be called.
//
// The OS wallets have no case of their own: each exists on one platform, where
// it is that platform's default, and the configuration layer never yields a
// name this system has not got.
func newSecretBackend(user string, log keys.Logger, settings config.Settings) (keys.SecretBackend, func()) {
	switch settings.SecretBackend {
	case config.SecretBackendOnePassword:
		return &keys.OnePasswordBackend{
			Runner:  keys.ExecRunner{Timeout: settings.CommandTimeout},
			Vault:   settings.OnePasswordVault,
			Timeout: settings.InteractiveTimeout,
		}, func() {}
	case config.SecretBackendBitwarden:
		return &keys.BitwardenBackend{
			Runner:   keys.ExecRunner{Timeout: settings.CommandTimeout},
			Prompter: newWalletPasswordPrompter(settings),
			Email:    settings.BitwardenEmail,
			Server:   settings.BitwardenServer,
			Timeout:  settings.InteractiveTimeout,
		}, func() {}
	case config.SecretBackendKeePassXC:
		return newKeePassXCBackend(runtime.GOOS, user, log, settings)
	default:
		return newDefaultSecretBackend(user, log, settings)
	}
}

// walletPasswordPrompter asks for a wallet's own password — Bitwarden's master
// password, a KeePassXC database's password — via the same graphical dialog as
// an SSH key's own passphrase when a display is available, otherwise on the
// controlling terminal. This is a separate prompt from the SSH key passphrase
// prompt: it never touches the wallet, and unlike a wallet-backed key's
// passphrase it cannot be silently skipped, so it needs a terminal fallback
// even where the SSH key prompt would just add the key on the terminal
// directly instead.
type walletPasswordPrompter struct {
	kdialog keys.KDialogPrompter
	gui     bool
}

func (p walletPasswordPrompter) Prompt(keyname string) (string, error) {
	if p.gui {
		return p.kdialog.Prompt(keyname)
	}
	// Terminal fallback: reaches the real controlling terminal, so it cannot run
	// in a unit test; the GUI branch above is unit-tested.
	//coverage:ignore
	return ttyPrompter{}.Prompt("Enter "+keyname, true)
}

func (walletPasswordPrompter) Available() bool { return true }

func newWalletPasswordPrompter(settings config.Settings) keys.Prompter {
	runner := keys.ExecRunner{Timeout: settings.CommandTimeout}
	kdialog := keys.KDialogPrompter{Runner: runner, Timeout: settings.InteractiveTimeout}
	return walletPasswordPrompter{kdialog: kdialog, gui: detectGUIAvailable()}
}

// detectGUIAvailable reports whether a graphical passphrase prompt can be shown
// in this environment, by the same test askpassEnv and doctor's askpass finding
// both rely on: a reachable display server and an installed prompter.
func detectGUIAvailable() bool {
	runner := keys.ExecRunner{}
	guiEnv := keys.GUIEnv{
		WaylandDisplay: os.Getenv("WAYLAND_DISPLAY"),
		Display:        os.Getenv("DISPLAY"),
	}
	return keys.GUIAvailable(guiEnv, runner, keys.KDialogPrompter{Runner: runner})
}
