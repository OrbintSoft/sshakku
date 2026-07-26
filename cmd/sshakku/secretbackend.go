package main

import (
	"fmt"
	"os"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys"
	"github.com/OrbintSoft/sshakku/internal/secretservice"
)

// newSecretBackend opens the secret backend settings.SecretBackend selects
// (config.toml's secret_backend key; secret-service is the default). For
// secret-service it opens the native Secret Service client and wraps it in a
// SecretServiceBackend, which unlocks its own dedicated collection only for
// the duration of each lookup/store rather than relying on the desktop's
// idle timeout. If the session bus is unreachable (e.g. a headless session
// with no D-Bus user session) it logs the failure and falls back to
// SecretToolBackend, so a key can still be looked up or stored via the
// desktop's default collection rather than aborting the caller outright.
// 1password and bitwarden shell out to their own CLI instead, so neither
// touches D-Bus. The returned func releases any resource newSecretBackend
// opened (only the Secret Service client today) and must always be called.
func newSecretBackend(user string, log keys.Logger, settings config.Settings) (keys.SecretBackend, func()) {
	switch settings.SecretBackend {
	case config.SecretBackendKeychain:
		return newKeychainBackend(user), func() {}
	case config.SecretBackendOnePassword:
		return &keys.OnePasswordBackend{Runner: keys.ExecRunner{}, Vault: settings.OnePasswordVault}, func() {}
	case config.SecretBackendBitwarden:
		return &keys.BitwardenBackend{
			Runner:   keys.ExecRunner{},
			Prompter: newBitwardenPrompter(),
			Email:    settings.BitwardenEmail,
			Server:   settings.BitwardenServer,
		}, func() {}
	default:
		client, err := secretservice.NewClient()
		if err != nil {
			_ = log.Log("ERROR", fmt.Sprintf("secret service: %v; falling back to secret-tool", err))
			return keys.SecretToolBackend{Runner: keys.ExecRunner{}, User: user}, func() {}
		}
		// Reached only when the session bus is live; the returned client is a
		// concrete *secretservice.Client that a unit test cannot stand in for
		// without a real D-Bus Secret Service, so this cannot run in a unit
		// test. The fallback above is unit-tested.
		//coverage:ignore
		return &keys.SecretServiceBackend{Client: client, User: user}, func() {
			//coverage:ignore
			_ = client.Close()
		}
	}
}

// bitwardenMasterPrompter asks for BitwardenBackend's master password: via the
// same graphical dialog as an SSH key's own passphrase when a display is
// available, otherwise on the controlling terminal. This is a separate prompt
// from the SSH key passphrase prompt — it never touches the wallet, and
// unlike a wallet-backed key's passphrase it cannot be silently skipped, so
// it needs a terminal fallback even where the SSH key prompt would just add
// the key on the terminal directly instead.
type bitwardenMasterPrompter struct {
	kdialog keys.KDialogPrompter
	gui     bool
}

func (p bitwardenMasterPrompter) Prompt(keyname string) (string, error) {
	if p.gui {
		return p.kdialog.Prompt(keyname)
	}
	// Terminal fallback: reaches the real controlling terminal, so it cannot run
	// in a unit test; the GUI branch above is unit-tested.
	//coverage:ignore
	return ttyPrompter{}.Prompt("Enter "+keyname, true)
}

func (bitwardenMasterPrompter) Available() bool { return true }

func newBitwardenPrompter() keys.Prompter {
	runner := keys.ExecRunner{}
	return bitwardenMasterPrompter{kdialog: keys.KDialogPrompter{Runner: runner}, gui: detectGUIAvailable()}
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
	case config.SecretBackendSecretService, config.SecretBackendOnePassword, config.SecretBackendBitwarden, config.SecretBackendKeychain:
		return true
	default:
		return false
	}
}
