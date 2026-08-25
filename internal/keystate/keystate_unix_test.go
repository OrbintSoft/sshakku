//go:build unix

package keystate

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSavedRecordIsPrivate checks the mode Save writes rather than trusting
// the constant.
//
// It asks the question where the answer means something. A permission bit is
// how a Unix system says who may open a file; a system that grants access by
// access-control list reports mode bits it synthesised, and 0600 is not
// something that can be asserted of one — nor would asserting it have checked
// anything about who may read the file there.
func TestSavedRecordIsPrivate(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir}
	require.NoError(t, s.Save("id_rsa", 8*time.Hour), "Save")

	info, err := os.Stat(filepath.Join(dir, "id_rsa"))
	require.NoError(t, err, "stat record")
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "record permissions")
}

// A store that cannot be listed must say so rather than answer with an empty
// list: a store nobody has written to yet has no records and that is not an
// error, and the two must not arrive as the same answer — read as "no keys",
// a key past its lifetime stays in the agent with nothing logged.
//
// The refusal is asked for with a regular file where the directory belongs,
// which is a question only this kind of system answers. Listing one here fails;
// on a system that opens a file as a directory and finds it empty, there is no
// refusal to be had and the caller is told there are no records.
func TestRecordsThatCannotBeListedAreNotAnEmptyStore(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))

	_, err := Store{Dir: file}.Records()

	assert.Error(t, err, "want an error when the store directory cannot be listed")
}
