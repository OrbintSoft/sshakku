//go:build darwin

package config

// SecretBackendKeychain is the operating system's own keychain — on macOS the
// one Security.framework reads and writes. It is declared here because it is
// what this build has instead of a freedesktop Secret Service, which no system
// outside Linux provides.
const SecretBackendKeychain = "keychain"

// platformSecretBackends are the wallets that can be chosen on this system, and
// platformDefaultSecretBackend the one used when the configuration names none.
//
// A name outside this list is not a wallet with a missing piece, it is a value
// that cannot mean anything here — see resolveSecretBackendFrom.
var platformSecretBackends = []string{
	SecretBackendKeychain,
	SecretBackendOnePassword,
	SecretBackendBitwarden,
	SecretBackendKeePassXC,
}

const platformDefaultSecretBackend = SecretBackendKeychain
