//go:build linux

package keys

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/run"
	"github.com/OrbintSoft/sshakku/internal/run/runtest"
)

func TestSecretToolLookup(t *testing.T) {
	t.Run("hit trims the trailing newline", func(t *testing.T) {
		r := runtest.NewRunner().On("secret-tool", runtest.Stdout("hunter2\n", 0))
		b := SecretToolBackend{Runner: r, User: "alice"}
		pass, found, err := b.Lookup(t.Context(), defaultServicePrefix+"-id_rsa")
		require.NoError(t, err, "a stored passphrase must come back")
		assert.True(t, found, "the item is in the wallet, so it must be reported found")
		assert.Equal(t, "hunter2", pass,
			"and the passphrase must be exactly what was stored: secret-tool's trailing newline is not part of it")
		require.NotEmpty(t, r.Calls, "the wallet must actually be asked")
		assert.Equal(t, []string{"lookup", "service", defaultServicePrefix + "-id_rsa", "username", "alice"},
			r.Calls[0].Args, "the lookup must name both the key and whose passphrase it is")
	})

	t.Run("miss is found=false, no error", func(t *testing.T) {
		r := runtest.NewRunner().On("secret-tool", runtest.Stdout("", 1))
		b := SecretToolBackend{Runner: r, User: "alice"}
		_, found, err := b.Lookup(t.Context(), defaultServicePrefix+"-id_rsa")
		require.NoError(t, err, "a passphrase that was never stored is not an error")
		assert.False(t, found, "and nothing may be reported found")
	})

	t.Run("a failure to start secret-tool is an error", func(t *testing.T) {
		wantErr := errors.New("boom")
		b := SecretToolBackend{Runner: runtest.NewRunner().On("secret-tool", runtest.Fails(wantErr)), User: "alice"}
		_, _, err := b.Lookup(t.Context(), "x")
		assert.ErrorIs(t, err, wantErr, "a wallet tool that would not run must be reported, not read as a miss")
	})
}

func TestSecretToolStore(t *testing.T) {
	const passphrase = "s3cr3t-pass"

	t.Run("passphrase goes on stdin, never in argv", func(t *testing.T) {
		r := runtest.NewRunner().On("secret-tool", runtest.Stdout("", 0))
		b := SecretToolBackend{Runner: r, User: "alice"}
		require.NoError(t, b.Store(t.Context(), defaultServicePrefix+"-id_rsa", "SSH Passphrase for id_rsa", passphrase),
			"saving a passphrase must succeed")
		require.NotEmpty(t, r.Calls, "the wallet must actually be asked")
		call := r.Calls[0]
		assert.Equal(t, passphrase, call.Stdin, "the passphrase must reach secret-tool out of sight, on standard input")
		for _, a := range call.Args {
			assert.NotContains(t, a, passphrase,
				"argv is world-readable on this machine: a passphrase there is readable by every other user")
		}
		require.GreaterOrEqualf(t, len(call.Args), 2, "the call must name what it is doing: %v", call.Args)
		assert.Equal(t, "store", call.Args[0], "and it must be a store")
		assert.Equal(t, "--label=SSH Passphrase for id_rsa", call.Args[1],
			"carrying the label, which is what a person sees in their wallet")
	})

	t.Run("a non-zero exit is an error", func(t *testing.T) {
		r := runtest.NewRunner().On("secret-tool", func(run.Cmd) (run.Result, error) {
			return run.Result{Stderr: []byte("no wallet"), Code: 1}, nil
		})
		b := SecretToolBackend{Runner: r, User: "alice"}
		assert.Error(t, b.Store(t.Context(), "x", "y", passphrase),
			"a passphrase the wallet refused to write must not be reported as saved")
	})
}

func TestSecretToolDelete(t *testing.T) {
	t.Run("clears the entry", func(t *testing.T) {
		r := runtest.NewRunner().On("secret-tool", runtest.Stdout("", 0))
		b := SecretToolBackend{Runner: r, User: "alice"}
		require.NoError(t, b.Delete(t.Context(), defaultServicePrefix+"-id_rsa"), "forgetting a passphrase must succeed")
		require.NotEmpty(t, r.Calls, "the wallet must actually be asked")
		assert.Equal(t, []string{"clear", "service", defaultServicePrefix + "-id_rsa", "username", "alice"},
			r.Calls[0].Args, "exactly the entry that was named may be cleared, and only this user's")
	})

	t.Run("a non-zero exit is an error", func(t *testing.T) {
		r := runtest.NewRunner().On("secret-tool", func(run.Cmd) (run.Result, error) {
			return run.Result{Code: 1}, nil
		})
		b := SecretToolBackend{Runner: r, User: "alice"}
		assert.Error(t, b.Delete(t.Context(), "x"), "a passphrase the wallet refused to remove must not be reported as forgotten")
	})

	t.Run("a failure to start secret-tool is an error", func(t *testing.T) {
		wantErr := errors.New("boom")
		b := SecretToolBackend{Runner: runtest.NewRunner().On("secret-tool", runtest.Fails(wantErr)), User: "alice"}
		assert.ErrorIs(t, b.Delete(t.Context(), "x"), wantErr,
			"a wallet tool that would not run must be reported, not read as a passphrase forgotten")
	})
}

func TestSecretToolList(t *testing.T) {
	b := SecretToolBackend{Runner: runtest.NewRunner(), User: "alice"}
	_, err := b.List(t.Context())
	assert.ErrorIs(t, err, ErrListUnsupported,
		"secret-tool can look an entry up but not enumerate one, and that must be said, not answered as empty")
}
