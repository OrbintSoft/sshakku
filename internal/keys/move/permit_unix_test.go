//go:build unix

package move

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Verifies F70 against real permissions rather than a stand-in. OpenSSH here
// refuses a private key that anyone else can read, and says so at the moment
// somebody is trying to connect rather than when the key was put there.

// modeOf is what the filesystem says about a path, rather than what was asked
// for.
func modeOf(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	return info.Mode().Perm()
}

func TestAPrivateKeyEndsUpReadableOnlyByItsOwner(t *testing.T) {
	key := filepath.Join(t.TempDir(), "id_ed25519")
	require.NoError(t, os.WriteFile(key, []byte("not really a key"), 0o644)) //nolint:gosec // G306: a key anyone can read is the precondition, and what this asserts is that it stops being one
	require.Equalf(t, os.FileMode(0o644), modeOf(t, key),
		"the precondition: anyone can read it to begin with, or this test proves nothing")

	require.NoError(t, Permit(key, PrivateKey))

	assert.Equal(t, os.FileMode(0o600), modeOf(t, key))
}

// TestAPublicHalfIsLeftPublic. It is a public key, and a tool reading it to
// work out which key to offer is doing its job.
func TestAPublicHalfIsLeftPublic(t *testing.T) {
	public := filepath.Join(t.TempDir(), "id_ed25519.pub")
	require.NoError(t, os.WriteFile(public, []byte("ssh-ed25519 AAAA"), 0o600))

	require.NoError(t, Permit(public, PublicKey))

	assert.Equal(t, os.FileMode(0o644), modeOf(t, public))
}

func TestAKeyDirectoryEndsUpEnterableOnlyByItsOwner(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "keys")
	require.NoError(t, os.Mkdir(dir, 0o755)) //nolint:gosec // G301: a directory anyone can enter is the precondition, and what this asserts is that it stops being one

	require.NoError(t, Permit(dir, Directory))

	assert.Equal(t, os.FileMode(0o700), modeOf(t, dir))
}

// TestAPathThatIsNotThereIsARefusalRatherThanASilence, so a run that could not
// permit a key stops instead of reporting that it did.
func TestAPathThatIsNotThereIsARefusalRatherThanASilence(t *testing.T) {
	err := Permit(filepath.Join(t.TempDir(), "nothing-here"), PrivateKey)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "nothing-here", "and names what it could not do")
}

// TestAKindNobodyDefinedIsRefused rather than quietly given mode zero, which is
// a key nothing can read at all.
func TestAKindNobodyDefinedIsRefused(t *testing.T) {
	key := filepath.Join(t.TempDir(), "id_ed25519")
	require.NoError(t, os.WriteFile(key, []byte("not really a key"), 0o600))

	require.Error(t, Permit(key, Kind(99)))
	assert.Equal(t, os.FileMode(0o600), modeOf(t, key), "and the file is left exactly as it was")
}
