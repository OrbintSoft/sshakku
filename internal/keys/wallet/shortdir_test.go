package wallet

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// shortDir returns a fresh temp dir short enough to hold a unix socket — not
// t.TempDir()'s nested, test-name-derived path, and not os.MkdirTemp("",
// ...)'s default either: on Darwin that resolves under $TMPDIR, itself a long
// per-boot randomized path (/var/folders/.../T/), and a test that spends its
// budget on the directory leading up to the socket has nothing left to say
// about the socket. A socket address is capped at barely a hundred bytes and
// is checked at bind, not at connect.
//
// Which directory is short enough is the platform's answer, not this
// function's: shortSocketBase gives it.
func shortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp(shortSocketBase(), "sshakku") //nolint:usetesting // t.TempDir() is the long macOS path the comment above is about
	require.NoError(t, err, "a short directory to put the socket in")
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}
