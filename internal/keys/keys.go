// Package keys loads the user's SSH keys into the agent: it enumerates the
// private keys in the key directory, skips any whose fingerprint is already in
// the agent,
// and adds the rest, pulling each passphrase from the OS secret store and handing
// it to ssh-add out of band. It never reimplements ssh-add or ssh-keygen — it
// drives the OpenSSH tools and the secret store through the seams below.
package keys

// EnvPassHandoffToken names the environment variable carrying the one-shot
// passphrase-handoff token from the loader to the askpass helper — a kernel
// keyring serial on Linux, a private Unix socket path on Darwin (see
// handoff_linux.go/handoff_darwin.go). Only the token — a handle — crosses
// the env; the passphrase itself never does.
const EnvPassHandoffToken = "SSHAKKU_HANDOFF_TOKEN"
