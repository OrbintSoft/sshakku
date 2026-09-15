//go:build windows

package protect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Verifies F69 against the real Encrypting File System rather than against a
// stand-in. The seam this exercises is the one a fake cannot stand in for: what
// the filesystem does with the attribute, and what a directory marked this way
// does to the files made in it afterwards. Everything happens in a directory
// the test makes and the test runner removes.

// TestARealDirectoryIsProtectedAndSaysSo drives the whole round on this system.
func TestARealDirectoryIsProtectedAndSaysSo(t *testing.T) {
	dir := t.TempDir()

	before, err := Protected(dir)
	require.NoError(t, err, "a directory that exists can be asked about")
	require.NotNil(t, before)
	require.Falsef(t, *before,
		"the precondition: a directory made just now is not protected, or this test proves nothing")

	if refused := Protect(dir); refused != nil {
		t.Skipf("this system would not protect a directory (%v) — EFS is absent on Home editions "+
			"and on filesystems other than NTFS, which is a property of the machine and not a failure here", refused)
	}

	after, err := Protected(dir)
	require.NoError(t, err)
	require.NotNil(t, after)
	assert.True(t, *after, "and afterwards it says so")
}

// TestAKeyMadeInAProtectedDirectoryIsBornProtected is why the directory is
// marked at all, and not only the keys in it: the key somebody generates
// tomorrow is covered without their running anything again.
func TestAKeyMadeInAProtectedDirectoryIsBornProtected(t *testing.T) {
	dir := t.TempDir()
	if err := Protect(dir); err != nil {
		t.Skipf("this system would not protect a directory: %v", err)
	}

	born := filepath.Join(dir, "id_later")
	require.NoError(t, os.WriteFile(born, []byte("not really a key"), 0o600))

	answer, err := Protected(born)
	require.NoError(t, err)
	require.NotNil(t, answer)
	assert.True(t, *answer, "a file made inside a protected directory is protected as it is made")
}

// TestAKeyAlreadyThereIsNotCoveredByMarkingTheDirectory is the reason the
// command hands every key over one by one. This is the failure the report has
// to catch, and it is asserted here against the real filesystem rather than
// taken from documentation.
func TestAKeyAlreadyThereIsNotCoveredByMarkingTheDirectory(t *testing.T) {
	dir := t.TempDir()
	already := filepath.Join(dir, "id_already")
	require.NoError(t, os.WriteFile(already, []byte("not really a key"), 0o600))

	if err := Protect(dir); err != nil {
		t.Skipf("this system would not protect a directory: %v", err)
	}

	answer, err := Protected(already)
	require.NoError(t, err)
	require.NotNil(t, answer)
	assert.Falsef(t, *answer,
		"marking a directory leaves what was already in it alone — if this ever becomes true, "+
			"the command may stop walking the keys, and until then it must not")
}

// TestAPathThatIsNotThereIsNotReportedUnprotected. A key file that has gone is
// not a key file lying about in the clear, and the two must not print the same.
func TestAPathThatIsNotThereIsNotReportedUnprotected(t *testing.T) {
	answer, err := Protected(filepath.Join(t.TempDir(), "nothing-here"))

	require.Error(t, err, "a path that cannot be read about is an error, not an answer")
	assert.Nil(t, answer)
}
