// Package keys loads the user's SSH keys into the agent: it enumerates the
// private keys in the key directory, skips any whose fingerprint is already in
// the agent, and adds the rest, pulling each passphrase from the OS secret
// store and handing it to ssh-add out of band. It never reimplements ssh-add or
// ssh-keygen — it drives the OpenSSH tools and the secret store through the
// seams below.
package keys
