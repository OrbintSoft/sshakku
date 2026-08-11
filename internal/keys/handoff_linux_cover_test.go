//go:build linux

package keys

import (
	"errors"
	"testing"
	"time"

	"github.com/OrbintSoft/sshakku/internal/keyring"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// saveKeyringHandoffSeams snapshots the keyring-op seams (and the shared RNG)
// used by the Linux passphrase handoff, restoring them when the (sub)test ends.
func saveKeyringHandoffSeams(t *testing.T) {
	t.Helper()
	oRand := randRead
	oAdd, oRead, oUnlink, oTimeout := keyringAdd, keyringRead, keyringUnlink, keyringSetTimeout
	t.Cleanup(func() {
		randRead = oRand
		keyringAdd, keyringRead, keyringUnlink, keyringSetTimeout = oAdd, oRead, oUnlink, oTimeout
	})
}

func TestStashPassphrase(t *testing.T) {
	t.Run("token RNG fails", func(t *testing.T) {
		saveKeyringHandoffSeams(t)
		randRead = func([]byte) (int, error) { return 0, errors.New("rng boom") }
		_, err := stashPassphrase("secret", time.Minute)
		assert.Error(t, err, "a key another process could guess the name of must not be added to the keyring")
	})

	t.Run("keyring add fails", func(t *testing.T) {
		saveKeyringHandoffSeams(t)
		keyringAdd = func(string, []byte) (keyring.Serial, error) { return 0, errors.New("EDQUOT") }
		_, err := stashPassphrase("secret", time.Minute)
		assert.Error(t, err,
			"with the passphrase nowhere the helper can collect it, ssh-add would prompt on a terminal nobody is watching")
	})

	t.Run("success returns the serial as a token and sets a timeout", func(t *testing.T) {
		saveKeyringHandoffSeams(t)
		var gotTTL time.Duration
		keyringAdd = func(string, []byte) (keyring.Serial, error) { return 4242, nil }
		keyringSetTimeout = func(_ keyring.Serial, d time.Duration) error { gotTTL = d; return nil }
		token, err := stashPassphrase("secret", 90*time.Second)
		require.NoError(t, err, "putting a passphrase aside must succeed")
		assert.Equal(t, "4242", token, "the handle handed to the helper is the key it will read back")
		assert.Equal(t, 90*time.Second, gotTTL,
			"and the kernel must be told when to drop it, or a passphrase nobody collected stays in the keyring")
	})
}

func TestFetchPassphrase(t *testing.T) {
	t.Run("malformed token", func(t *testing.T) {
		_, err := fetchPassphrase("not-a-serial")
		assert.Error(t, err, "a handle no stash was made under can redeem nothing")
	})

	t.Run("keyring read fails, key still unlinked", func(t *testing.T) {
		saveKeyringHandoffSeams(t)
		unlinked := false
		keyringRead = func(keyring.Serial) ([]byte, error) { return nil, errors.New("EKEYEXPIRED") }
		keyringUnlink = func(keyring.Serial) error { unlinked = true; return nil }
		_, err := fetchPassphrase("7")
		assert.Error(t, err, "a passphrase that could not be read must be reported, not handed on as an empty one")
		assert.True(t, unlinked,
			"and the key must go anyway: a passphrase left in the keyring after a failed collection is one nobody is watching")
	})

	t.Run("round trip reads then unlinks the key", func(t *testing.T) {
		saveKeyringHandoffSeams(t)
		var unlinked keyring.Serial
		keyringRead = func(keyring.Serial) ([]byte, error) { return []byte("s3cr3t"), nil }
		keyringUnlink = func(s keyring.Serial) error { unlinked = s; return nil }
		got, err := fetchPassphrase("7")
		require.NoError(t, err, "collecting a stashed passphrase must succeed")
		assert.Equal(t, "s3cr3t", got, "and hand back exactly what was put aside")
		assert.Equal(t, keyring.Serial(7), unlinked,
			"then remove exactly that key: one stash is one handoff, and what is left behind is readable")
	})
}
