// Package testtmp hands a test a temporary directory short enough to bind a
// unix socket in.
//
// It exists because `t.TempDir()` cannot be used for one. A socket address is
// capped at barely a hundred bytes and the cap is checked at bind, not at
// connect, so a test that spends that budget on the path leading up to the
// socket has nothing left to say about the socket — and `t.TempDir()` nests
// the (sub)test's own name under the OS temp root, which on macOS is itself a
// long per-boot randomised path (`/var/folders/…/T/`). `os.MkdirTemp("", …)`
// resolves to that same root and is no better.
//
// Which directory is short enough is the platform's answer, not this
// package's; `socketBase` gives it.
package testtmp

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// ShortDir returns a fresh directory under the platform's short base, removed
// when the test ends.
func ShortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp(socketBase(), "sshakku") //nolint:usetesting // t.TempDir() is the long path this package exists to avoid
	require.NoError(t, err, "a short directory to put the socket in")
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}
