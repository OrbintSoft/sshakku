package giveup

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRecordMkdirError covers Record's failure to create Dir: when a parent path
// component is a regular file, MkdirAll cannot make the directory and the error
// is returned rather than swallowed.
func TestRecordMkdirError(t *testing.T) {
	base := t.TempDir()
	file := filepath.Join(base, "not-a-dir")
	require.NoError(t, os.WriteFile(file, nil, 0o600), "seed")
	s := Store{Dir: filepath.Join(file, "sub"), TTL: time.Hour}
	assert.Error(t, s.Record("id_rsa"), "want an error when Dir cannot be created")
}

// TestClearRemoveError covers Clear's error return for a removal failure that is
// not "already absent": a sentinel that is a non-empty directory cannot be
// unlinked, so the error propagates instead of being treated as a missing record.
func TestClearRemoveError(t *testing.T) {
	dir := t.TempDir()
	sentinel := filepath.Join(dir, "id_rsa")
	require.NoError(t, os.Mkdir(sentinel, 0o700), "seed dir")
	require.NoError(t, os.WriteFile(filepath.Join(sentinel, "child"), nil, 0o600), "seed child")
	s := Store{Dir: dir, TTL: time.Hour}
	assert.Error(t, s.Clear("id_rsa"), "want an error when the sentinel cannot be removed")
}

// TestUnusableKeyNameDisables covers path's rejection of key names that reduce to
// an unusable base ("..", "/", or all-whitespace): every operation becomes a
// no-op rather than letting a crafted name escape Dir.
func TestUnusableKeyNameDisables(t *testing.T) {
	s := Store{Dir: t.TempDir(), TTL: time.Hour}
	for _, key := range []string{"..", "/", "   "} {
		assert.Falsef(t, s.GivenUp(key), "GivenUp(%q) must be false for an unusable key", key)
		assert.NoErrorf(t, s.Record(key), "Record(%q)", key)
		assert.NoErrorf(t, s.Clear(key), "Clear(%q)", key)
	}
}
