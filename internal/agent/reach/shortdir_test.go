package reach

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// shortDir returns a fresh, auto-cleaned temp directory with a short path.
// Unlike t.TempDir(), which nests the (sub)test name under the OS temp root
// (e.g. macOS's /var/folders/xx/.../T/TestName.../001/), it stays well under
// the 104-byte sun_path limit unix sockets are bound under on BSD/Darwin —
// a limit t.TempDir()'s deeper macOS layout routinely exceeds.
func shortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "sshakku") //nolint:usetesting // t.TempDir() is the long macOS path the comment above is about
	require.NoError(t, err, "mkdir temp")
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}
