package main

import (
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
			Runner:        keys.ExecRunner{Timeout: settings.CommandTimeout},
			Prompter:      newWalletPasswordPrompter(settings),
			Email:         settings.BitwardenEmail,
			Server:        settings.BitwardenServer,
			Timeout:       settings.InteractiveTimeout,
			ServicePrefix: settings.ServicePrefix,
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
	// graphical is the platform's dialog, or nil where this build has none.
	graphical keys.Prompter
}

func (p walletPasswordPrompter) Prompt(keyname string) (string, error) {
	if p.graphical != nil {
		return p.graphical.Prompt(keyname)
	}
	// Terminal fallback: reaches the real controlling terminal, so it cannot run
	// in a unit test; the graphical branch above is unit-tested.
	//coverage:ignore
	return ttyPrompter{}.Prompt("Enter "+keyname, true)
}

func (walletPasswordPrompter) Available() bool { return true }

func newWalletPasswordPrompter(settings config.Settings) keys.Prompter {
	return walletPasswordPrompter{graphical: newGraphicalPrompter(settings)}
}
