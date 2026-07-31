package main

import (
	"os"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys"
)

// newSecretBackend opens the secret backend settings.SecretBackend selects
// (config.toml's secret_backend key; secret-service is the default).
// 1password and bitwarden shell out to their own CLI, keychain uses the OS
// keychain, and the default is resolved per-OS by newDefaultSecretBackend
// (the freedesktop Secret Service on Linux, the keychain off it). The returned
// func releases any resource the backend opened and must always be called.
func newSecretBackend(user string, log keys.Logger, settings config.Settings) (keys.SecretBackend, func()) {
	switch settings.SecretBackend {
	case config.SecretBackendKeychain:
		return newKeychainBackend(user), func() {}
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
		return newKeePassXCBackend(user, log, settings)
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

// validSecretBackendName reports whether name is one of the secret backends
// newSecretBackend knows how to construct.
func validSecretBackendName(name string) bool {
	switch name {
	case config.SecretBackendSecretService, config.SecretBackendOnePassword, config.SecretBackendBitwarden,
		config.SecretBackendKeychain, config.SecretBackendKeePassXC:
		return true
	default:
		return false
	}
}
