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
// The wallets reached by running a program of their own are absent on purpose.
// They compile here, which is not the same as having been shown to work here,
// and what is offered is what has been driven on this system.
var platformSecretBackends = []string{
	SecretBackendCredentialManager,
}

const platformDefaultSecretBackend = SecretBackendCredentialManager
