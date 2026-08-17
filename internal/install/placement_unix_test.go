//go:build unix

package install

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// A name this system cannot resolve at all — a directory sought under a file —
// is neither there nor absent, and is reported rather than read as absent. That
// distinction is this system's own: elsewhere the same name comes back as simply
// not found, which is an answer the caller may act on.
func TestAPathThatCannotBeReachedIsNotReadAsAbsent(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(file, []byte("mine"), 0o644))

	_, err := isDir(filepath.Join(file, "under-a-file"))

	require.Error(t, err)
}
