//go:build unix

package giveup

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRecordSentinelIsPrivate checks the mode Record writes rather than
// trusting the constant.
//
// It asks the question where the answer means something. A permission bit is
// how a Unix system says who may open a file; a system that grants access by
// access-control list reports mode bits it synthesised, and 0600 is not
// something that can be asserted of one — nor would asserting it have checked
// anything about who may read the file there. What that system's own answer
// is, and what checks it, is still owed: paths.PrivateDir says the same of a
// directory and reports false rather than guessing.
func TestRecordSentinelIsPrivate(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir, TTL: time.Hour}
	require.NoError(t, s.Record("id_rsa"), "Record")

	info, err := os.Stat(filepath.Join(dir, "id_rsa"))
	require.NoError(t, err, "stat sentinel")
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "sentinel permissions")
}
