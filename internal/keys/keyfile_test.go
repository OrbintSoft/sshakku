package keys

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// encryptedKeyFile writes a real ed25519 private key, locked with passphrase,
// into a file of the test's own and returns its path. Real key material rather
// than a stand-in: what is under test is whether a passphrase opens a key, and
// a fixture that cannot be opened at all would answer that question by
// construction.
func encryptedKeyFile(t *testing.T, passphrase string) string {
	t.Helper()
	return keyFileWith(t, "id_ed25519", func(key ed25519.PrivateKey) *pem.Block {
		block, err := ssh.MarshalPrivateKeyWithPassphrase(key, "", []byte(passphrase))
		require.NoError(t, err, "the test needs a key it can lock")
		return block
	})
}

// plainKeyFile writes a real ed25519 private key with no passphrase on it.
func plainKeyFile(t *testing.T) string {
	t.Helper()
	return keyFileWith(t, "id_plain", func(key ed25519.PrivateKey) *pem.Block {
		block, err := ssh.MarshalPrivateKey(key, "")
		require.NoError(t, err, "the test needs a key it can write")
		return block
	})
}

func keyFileWith(t *testing.T, name string, marshal func(ed25519.PrivateKey) *pem.Block) string {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err, "the test needs a key")
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, pem.EncodeToMemory(marshal(key)), 0o600))
	return path
}

// TestPassphraseVerdict covers the three answers the question can have, and
// most of all that the third one exists. Anything this build cannot open for
// itself — a format it does not implement, a path it cannot read — must come
// back as "could not tell" and never as "wrong": acting on that as though the
// passphrase were wrong would withhold an entry that works.
func TestPassphraseVerdict(t *testing.T) {
	const right = "the-right-one"

	locked := encryptedKeyFile(t, right)
	plain := plainKeyFile(t)

	notAKey := filepath.Join(t.TempDir(), "known_hosts")
	require.NoError(t, os.WriteFile(notAKey, []byte("host ssh-ed25519 AAAA\n"), 0o600))

	tests := []struct {
		name       string
		keyfile    string
		passphrase string
		want       passphraseVerdict
	}{
		{"the passphrase that locked it", locked, right, verdictOpens},
		{"a passphrase that does not", locked, "the-typo", verdictWrong},
		{"a key with no passphrase on it", plain, right, verdictUnknown},
		{"a file that is not a key", notAKey, right, verdictUnknown},
		{"a file that is not there", filepath.Join(t.TempDir(), "gone"), right, verdictUnknown},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, passphraseOpens(tc.keyfile, tc.passphrase),
				"what is done about a passphrase rests entirely on this answer")
		})
	}
}
