//go:build linux

package keys

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSecretToolStoreNonZeroExit covers Store's non-zero-exit branch: a failing
// secret-tool store is reported as an error carrying its stderr.
func TestSecretToolStoreNonZeroExit(t *testing.T) {
	r := newFakeRunner().on("secret-tool", func(Cmd) (Result, error) {
		return Result{Code: 1, Stderr: []byte("store denied")}, nil
	})
	b := SecretToolBackend{Runner: r, User: "u"}
	assert.Error(t, b.Store("svc", "label", "pass"),
		"a passphrase the wallet refused to write must not be reported as saved")
}

// TestSecretToolStoreRunError covers Store's start-failure branch: secret-tool
// cannot be run at all, which propagates as an error.
func TestSecretToolStoreRunError(t *testing.T) {
	boom := errors.New("secret-tool exec boom")
	r := newFakeRunner().on("secret-tool", fails(boom))
	b := SecretToolBackend{Runner: r, User: "u"}
	assert.ErrorIs(t, b.Store("svc", "label", "pass"), boom,
		"a wallet tool that would not run must be reported, not read as a passphrase saved")
}
