package keystate

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveMkdirAllError(t *testing.T) {
	base := t.TempDir()
	file := filepath.Join(base, "not-a-dir")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))
	// A component of Dir is a regular file, so creating the store directory fails.
	s := Store{Dir: filepath.Join(file, "sub")}
	assert.Error(t, s.Save("id_rsa", time.Hour), "want an error when the store directory cannot be created")
}

func TestRecordsReadDirError(t *testing.T) {
	base := t.TempDir()
	file := filepath.Join(base, "not-a-dir")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))
	// Dir is a regular file, so listing it fails with something other than "not
	// exist". A store that has never been written to has no records and that is
	// not an error; one that cannot be read is not the same answer, and must not
	// be handed back as an empty list.
	s := Store{Dir: file}
	_, err := s.Records()
	assert.Error(t, err, "want an error when the store directory cannot be listed")
}

func TestLoadWrongLineCountMisses(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "id_rsa"), []byte("only-one-line\n"), 0o600))
	_, ok := s.Load("id_rsa")
	assert.False(t, ok, "a record without exactly two lines must miss")
}

func TestLoadNonNumericLifetimeMisses(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir}
	// A valid timestamp so parsing reaches the lifetime, which is not a number.
	body := "2026-07-08T12:00:00Z\nnot-a-number\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "id_rsa"), []byte(body), 0o600))
	_, ok := s.Load("id_rsa")
	assert.False(t, ok, "a record with a non-numeric lifetime must miss")
}

func TestClearRemoveError(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir}
	// The record path is a non-empty directory, so os.Remove fails with an error
	// other than "not exist", which Clear must propagate.
	recDir := filepath.Join(dir, "id_rsa")
	require.NoError(t, os.Mkdir(recDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(recDir, "child"), []byte("x"), 0o600))
	assert.Error(t, s.Clear("id_rsa"), "want an error when the record cannot be removed")
}

func TestUnusableKeyIsIgnored(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir}
	// Keys whose base name is unusable must be rejected before any file I/O.
	for _, key := range []string{".", "..", string(filepath.Separator), "   "} {
		assert.NoErrorf(t, s.Save(key, time.Hour), "Save(%q)", key)
		_, ok := s.Load(key)
		assert.Falsef(t, ok, "Load(%q) must miss for an unusable key", key)
		assert.NoErrorf(t, s.Clear(key), "Clear(%q)", key)
	}
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "no record should be written for unusable keys")
}
