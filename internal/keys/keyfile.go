package keys

import (
	"crypto/x509"
	"errors"
	"os"

	"golang.org/x/crypto/ssh"
)

// passphraseVerdict is what trying to open a key file with a passphrase
// settled. It has three values rather than two because the question can go
// unanswered, and an unanswered question is not a "no": a key in a format this
// build does not implement, or a path this process cannot read, tells us
// nothing about the passphrase — and acting on that as though the passphrase
// were wrong would refuse one that works.
type passphraseVerdict int

const (
	// verdictUnknown is the answer whenever the key itself could not be
	// reached: it must leave the caller doing exactly what it did before.
	verdictUnknown passphraseVerdict = iota
	verdictOpens
	verdictWrong
)

// passphraseOpens reports whether passphrase decrypts the private key in
// keyfile. The work is done here rather than by running ssh-keygen because a
// passphrase must never reach a command line, where anyone on the machine can
// read it out of the process table.
//
// Only the store's own "this is the wrong password" is taken for a wrong
// passphrase. Every other failure — a file that is not a key, a format without
// an implementation here, a path that does not resolve in this process the way
// it did in the one that printed the prompt — is a question that was not
// answered.
func passphraseOpens(keyfile, passphrase string) passphraseVerdict {
	// The path is the one ssh printed in its own prompt, and names a key file
	// ssh is reading as this user at this moment.
	pemBytes, err := os.ReadFile(keyfile)
	if err != nil {
		return verdictUnknown
	}
	if _, err := ssh.ParseRawPrivateKeyWithPassphrase(pemBytes, []byte(passphrase)); err != nil {
		if errors.Is(err, x509.IncorrectPasswordError) {
			return verdictWrong
		}
		return verdictUnknown
	}
	return verdictOpens
}
