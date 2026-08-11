//go:build windows

package config

// platformSecretBackends are the wallets that can be chosen on this system, and
// platformDefaultSecretBackend the one used when the configuration names none.
//
// Neither is anything yet. This system's own wallet is the Credential Manager,
// which sshakku cannot read or write here; and the wallets that are reached
// through a tool of their own rather than an OS API are not offered either,
// because being compilable here is not the same as having been shown to work
// here. An empty list is what says both: nothing can be chosen, and nothing is
// silently in force.
var platformSecretBackends []string

const platformDefaultSecretBackend = ""
