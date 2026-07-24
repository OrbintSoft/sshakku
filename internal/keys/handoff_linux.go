//go:build linux

package keys

import (
	"fmt"
	"strconv"
	"time"

	"github.com/OrbintSoft/sshakku/internal/keyring"
)

// stashPassphrase stores passphrase in the @u kernel keyring under a random
// description, sets it to expire after ttl, and returns the keyring serial
// (as a string) — the handoff token fetchPassphrase later redeems.
func stashPassphrase(passphrase string, ttl time.Duration) (string, error) {
	desc, err := randomHandoffToken()
	if err != nil {
		return "", err
	}
	serial, err := keyring.Add("sshakku-pass-"+desc, []byte(passphrase))
	if err != nil {
		return "", err
	}
	_ = keyring.SetTimeout(serial, ttl)
	return strconv.Itoa(int(serial)), nil
}

// fetchPassphrase reads and unlinks the keyring entry token names — a
// one-shot read, whether or not it succeeds, so a leaked passphrase cannot
// linger in the keyring.
func fetchPassphrase(token string) (string, error) {
	n, err := strconv.Atoi(token)
	if err != nil {
		return "", fmt.Errorf("malformed handoff token %q: %w", token, err)
	}
	serial := keyring.Serial(n)
	pass, err := keyring.Read(serial)
	_ = keyring.Unlink(serial)
	if err != nil {
		return "", err
	}
	return string(pass), nil
}
