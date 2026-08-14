package inspect

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReadStatusUIDMalformed covers readStatusUID's fallbacks for a status file
// whose Uid line is missing, has too few fields, or is non-numeric — shapes the
// real /proc never produces, so they are only reachable through a crafted file.
func TestReadStatusUIDMalformed(t *testing.T) {
	write := func(t *testing.T, body string) string {
		t.Helper()
		p := filepath.Join(t.TempDir(), "status")
		require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
		return p
	}

	t.Run("Uid line with too few fields", func(t *testing.T) {
		assert.Equal(t, -1, readStatusUID(write(t, "Name:\tx\nUid:\n")), "a short Uid line leaves the owner unknown")
	})
	t.Run("non-numeric uid", func(t *testing.T) {
		assert.Equal(t, -1, readStatusUID(write(t, "Uid:\tnobody\tnobody\n")), "a non-numeric uid leaves the owner unknown")
	})
	t.Run("no Uid line at all", func(t *testing.T) {
		assert.Equal(t, -1, readStatusUID(write(t, "Name:\tx\nState:\tS\n")), "no Uid line leaves the owner unknown")
	})
}

// TestReadProcfsTreeUnreadableCmdline covers readCmdline's read-failure path: a pid
// directory with no cmdline file is skipped rather than reported as an error.
func TestReadProcfsTreeUnreadableCmdline(t *testing.T) {
	root := t.TempDir()
	// A pid directory that exists but has no cmdline file (the process vanished
	// mid-scan). ReadFile fails and the entry is skipped.
	require.NoError(t, os.Mkdir(filepath.Join(root, "999"), 0o755))
	procs, err := readProcfsTree(root)
	require.NoError(t, err, "readProcfsTree")
	assert.Empty(t, procs, "the cmdline-less entry must be skipped")
}
