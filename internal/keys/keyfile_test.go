package keys

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

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

func keyFileWith(t *testing.T, name string, marshal func(ed25519.PrivateKey) *pem.Block) string {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err, "the test needs a key")
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, pem.EncodeToMemory(marshal(key)), 0o600))
	return path
}
