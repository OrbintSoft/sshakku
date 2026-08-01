//go:build linux

package config

// SecretBackendSecretService is the freedesktop Secret Service, the wallet API
// a Linux desktop provides over the session bus (GNOME Keyring, KWallet and
// KeePassXC all answer it). It is declared here because it is a Linux
// mechanism: a build for another operating system has no such thing to name,
// and a user there cannot choose it.
const SecretBackendSecretService = "secret-service"

// platformSecretBackends are the wallets that can be chosen on this system, and
// platformDefaultSecretBackend the one used when the configuration names none.
//
// A name outside this list is not a wallet with a missing piece, it is a value
// that cannot mean anything here — see resolveSecretBackendFrom.
var platformSecretBackends = []string{
	SecretBackendSecretService,
	SecretBackendOnePassword,
	SecretBackendBitwarden,
	SecretBackendKeePassXC,
}

const platformDefaultSecretBackend = SecretBackendSecretService
