//go:build linux

package keys

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/OrbintSoft/sshakku/internal/keyring"
)

// Seams over the kernel-keyring operations, so the stash and fetch error and
// success branches are exercisable without a live session keyring (unavailable
// in many containers and CI runners). Production points them at the real ops.
var (
	keyringAdd        = keyring.Add
	keyringRead       = keyring.Read
	keyringUnlink     = keyring.Unlink
	keyringSetTimeout = keyring.SetTimeout
)

// stashPassphrase stores passphrase in the @u kernel keyring under a random
// description, sets it to expire after ttl, and returns the keyring serial
// (as a string) — the handoff token fetchPassphrase later redeems.
func stashPassphrase(passphrase string, ttl time.Duration) (string, error) {
	desc, err := randomHandoffToken()
	if err != nil {
		return "", err
	}
	serial, err := keyringAdd("sshakku-pass-"+desc, []byte(passphrase))
	if err != nil {
		return "", err
	}
	_ = keyringSetTimeout(serial, ttl)
	return strconv.Itoa(int(serial)), nil
}

// fetchPassphrase reads and unlinks the keyring entry token names — a
// one-shot read, whether or not it succeeds, so a leaked passphrase cannot
// linger in the keyring.
func fetchPassphrase(_ context.Context, token string) (string, error) {
	n, err := strconv.Atoi(token)
	if err != nil {
		return "", fmt.Errorf("malformed handoff token %q: %w", token, err)
	}
	serial := keyring.Serial(n)
	pass, err := keyringRead(serial)
	_ = keyringUnlink(serial)
	if err != nil {
		return "", err
	}
	return string(pass), nil
}
