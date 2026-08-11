package keys

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnumeratorKeys(t *testing.T) {
	dir := t.TempDir()
	// Regular key files we expect to find.
	for _, name := range []string{"id_ed25519", "id_rsa"} {
		writeFile(t, filepath.Join(dir, name))
	}
	// Files we must skip: public keys, unrelated files, and a non-id_ name.
	for _, name := range []string{"id_ed25519.pub", "id_rsa.pub", "config", "known_hosts", "authorized_keys"} {
		writeFile(t, filepath.Join(dir, name))
	}
	// A subdirectory named like a key must be skipped (not a regular file).
	require.NoError(t, os.Mkdir(filepath.Join(dir, "id_dir"), 0o700), "seed a directory named like a key")
	// A symlink named like a key must be skipped (matches `find -type f`).
	if runtime.GOOS != "windows" {
		require.NoError(t, os.Symlink(filepath.Join(dir, "id_rsa"), filepath.Join(dir, "id_link")),
			"seed a symlink named like a key")
	}

	got, err := Enumerator{Dir: dir}.Keys()
	require.NoError(t, err, "listing a key directory must succeed")
	assert.Equal(t, []string{filepath.Join(dir, "id_ed25519"), filepath.Join(dir, "id_rsa")}, got,
		"the private keys and only those: a public half, a config file or a directory offered to ssh-add "+
			"spends the user's attention on failures no passphrase can fix")
}

func TestEnumeratorMissingDir(t *testing.T) {
	got, err := Enumerator{Dir: filepath.Join(t.TempDir(), "no-such-dir")}.Keys()
	require.NoError(t, err, "an account with no ~/.ssh at all is ordinary, not an error")
	assert.Empty(t, got, "and it has no keys")
}

// TestDefaultKeyPatternsIsTheRuleAndCannotBeChanged covers the naming rule a
// caller has to state rather than apply — F34's report shows what is in force,
// and "nothing" is not what is in force when no patterns are configured. It is
// handed out by value: a caller that edits what it was given must not change
// what the next one is told, or the report and the enumerator come to disagree
// about a rule neither of them was configured with.
func TestDefaultKeyPatternsIsTheRuleAndCannotBeChanged(t *testing.T) {
	got := DefaultKeyPatterns()
	require.NotEmpty(t, got, "the report has to show what is in force, and \"nothing\" is not what is in force")

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "id_rsa"))
	keys, err := Enumerator{Dir: dir, Patterns: got}.Keys()
	require.NoError(t, err, "listing a key directory must succeed")
	assert.Len(t, keys, 1,
		"and the rule handed out must match what the enumerator matches with no patterns at all, "+
			"or the report describes a rule nothing applies")

	got[0] = "changed"
	again := DefaultKeyPatterns()
	assert.NotEqual(t, "changed", again[0],
		"a caller that edits what it was given must not change what the next one is told")
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	require.NoErrorf(t, os.WriteFile(path, []byte("x"), 0o600), "seed %s", path)
}
