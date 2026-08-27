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
// 1Password is offered on the same terms, having been driven here against a
// real account. Naming it is the whole of what choosing it takes: op is asked
// for the vault by name, and nothing of this system's that another spells
// differently is involved.
//
// Bitwarden is still absent, and on those terms too: it compiles here, which is
// not the same as having been shown to work here, and what is offered is what
// has been driven on this system.
var platformSecretBackends = []string{
	SecretBackendCredentialManager,
	SecretBackendKeePassXC,
	SecretBackendOnePassword,
}

const platformDefaultSecretBackend = SecretBackendCredentialManager
