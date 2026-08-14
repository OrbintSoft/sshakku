//go:build unix

package sessionlog

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLogPerm checks the mode the log is created with rather than trusting the
// constant: the log records what happened to a user's keys and is nobody
// else's to read.
//
// It asks the question where the answer means something. A permission bit is
// how a Unix system says who may open a file; a system that grants access by
// access-control list reports mode bits it synthesised, and 0600 is not
// something that can be asserted of one — nor would asserting it have checked
// anything about who may read the file there.
func TestLogPerm(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.log")
	require.NoError(t, New(path).Log("INFO", "x"))
	fi, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), fi.Mode().Perm(), "log permissions")
}
