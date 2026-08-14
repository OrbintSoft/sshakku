//go:build linux

package keys

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/OrbintSoft/sshakku/internal/run"
	"github.com/OrbintSoft/sshakku/internal/run/runtest"
)

// TestSecretToolStoreNonZeroExit covers Store's non-zero-exit branch: a failing
// secret-tool store is reported as an error carrying its stderr.
func TestSecretToolStoreNonZeroExit(t *testing.T) {
	r := runtest.NewRunner().On("secret-tool", func(run.Cmd) (run.Result, error) {
		return run.Result{Code: 1, Stderr: []byte("store denied")}, nil
	})
	b := SecretToolBackend{Runner: r, User: "u"}
	assert.Error(t, b.Store(t.Context(), "svc", "label", "pass"),
		"a passphrase the wallet refused to write must not be reported as saved")
}

// TestSecretToolStoreRunError covers Store's start-failure branch: secret-tool
// cannot be run at all, which propagates as an error.
func TestSecretToolStoreRunError(t *testing.T) {
	boom := errors.New("secret-tool exec boom")
	r := runtest.NewRunner().On("secret-tool", runtest.Fails(boom))
	b := SecretToolBackend{Runner: r, User: "u"}
	assert.ErrorIs(t, b.Store(t.Context(), "svc", "label", "pass"), boom,
		"a wallet tool that would not run must be reported, not read as a passphrase saved")
}
