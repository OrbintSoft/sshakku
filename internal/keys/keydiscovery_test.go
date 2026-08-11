package keys

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEnumeratorHonoursPatterns covers the half of F34 that decides which names
// in the directory are keys: a key called something of the user's own is loaded
// like any other once the rule says so, and the rule SSHakku ships stays in
// force for anyone who says nothing.
func TestEnumeratorHonoursPatterns(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"id_ed25519", "work-github", "deploy", "id_ed25519.pub", "work-github.pub"} {
		writeFile(t, filepath.Join(dir, name))
	}

	t.Run("a name of the user's own is a key when the patterns say so", func(t *testing.T) {
		got := mustKeys(t, Enumerator{Dir: dir, Patterns: []string{"id_*", "work-*"}})
		assertNames(t, got, "id_ed25519", "work-github")
	})

	t.Run("naming no patterns keeps OpenSSH's own convention", func(t *testing.T) {
		got := mustKeys(t, Enumerator{Dir: dir})
		assertNames(t, got, "id_ed25519")
	})

	t.Run("a pattern matches the file name, not the path", func(t *testing.T) {
		got := mustKeys(t, Enumerator{Dir: dir, Patterns: []string{"deploy"}})
		assertNames(t, got, "deploy")
	})
}

// TestEnumeratorNeverOffersWhatIsNotAKey covers the promise F34 makes about the
// widest rule a user can write: "*" means every key in the directory, not every
// file in it. The public halves and the files OpenSSH keeps there for itself
// are not keys, and offering them to ssh-add would spend the user's attention
// on failures they cannot fix by typing a passphrase.
func TestEnumeratorNeverOffersWhatIsNotAKey(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{
		"id_ed25519", "id_ed25519.pub",
		"config", "known_hosts", "known_hosts.old",
		"authorized_keys", "authorized_keys2", "environment", "rc",
	} {
		writeFile(t, filepath.Join(dir, name))
	}

	got := mustKeys(t, Enumerator{Dir: dir, Patterns: []string{"*"}})
	assertNames(t, got, "id_ed25519")
}

// TestEnumeratorMissingDir covers the difference F34 draws between a directory
// nobody asked for and one the user named: the first is an ordinary account
// with no keys, the second is a mistake that looks exactly like it.
func TestEnumeratorMissingConfiguredDir(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such-dir")

	_, err := Enumerator{Dir: missing, MustExist: true}.Keys()
	require.Error(t, err,
		"a directory the user named and that is not there is a mistake, not an account with no keys")
	assert.Contains(t, err.Error(), missing, "and the report must name the directory that is not there")
}

func mustKeys(t *testing.T, e Enumerator) []string {
	t.Helper()
	got, err := e.Keys()
	require.NoError(t, err, "listing a key directory must succeed")
	return got
}

func assertNames(t *testing.T, paths []string, want ...string) {
	t.Helper()
	var got []string
	for _, p := range paths {
		got = append(got, filepath.Base(p))
	}
	assert.Equal(t, want, got, "exactly these files are keys, in this order")
}
