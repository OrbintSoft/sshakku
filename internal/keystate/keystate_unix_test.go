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
