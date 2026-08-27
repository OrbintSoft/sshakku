//go:build windows

package config

// SecretBackendCredentialManager is the store this operating system keeps for
// itself, holding a generic credential per entry under the account that saved
// it. It is declared here because it is this system's mechanism: a build for
// another operating system has no such thing to name, and a user there cannot
// choose it.
const SecretBackendCredentialManager = "credential-manager"

// platformSecretBackends are the wallets that can be chosen on this system, and
// platformDefaultSecretBackend the one used when the configuration names none.
//
// A name outside this list is not a wallet with a missing piece, it is a value
// that cannot mean anything here — see resolveSecretBackendFrom.
//
// KeePassXC is offered because it has been driven here, against a real
// keepassxc-cli and a real database, and not because it compiles. It is reached
// by opening the database file: the protocol a running KeePassXC serves is a
// named pipe on this system, which SSHakku has no dialler for, so that route
// reports itself unavailable rather than being chosen.
//
// The wallets reached by running a program of their own — 1Password, Bitwarden
// — are still absent, on the same terms: they compile here, which is not the
// same as having been shown to work here, and what is offered is what has been
// driven on this system.
var platformSecretBackends = []string{
	SecretBackendCredentialManager,
	SecretBackendKeePassXC,
}

const platformDefaultSecretBackend = SecretBackendCredentialManager
