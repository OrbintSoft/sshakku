package paths

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAbsent covers the distinction Absent exists to draw, on whichever system
// is running it: what is not there, and what is there but is not what the
// caller wanted. The second is the one platforms disagree about — see Absent —
// so a regular file and a directory both have to answer "something is here".
func TestAbsent(t *testing.T) {
	dir := t.TempDir()

	file := filepath.Join(dir, "a-file")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600), "seed a file")
	nested := filepath.Join(dir, "a-dir")
	require.NoError(t, os.Mkdir(nested, 0o700), "seed a directory")

	assert.True(t, Absent(filepath.Join(dir, "no-such-thing")),
		"nothing at the path is what most accounts start with, and is no error")
	assert.True(t, Absent(filepath.Join(dir, "no-such-dir", "nor-this")),
		"and neither is a path whose parent is not there either")
	assert.False(t, Absent(file),
		"a regular file is something that is there, whatever the error a caller got trying to read it as a directory")
	assert.False(t, Absent(nested), "so is a directory")
	assert.False(t, Absent(dir), "and so is the one it is all in")
}
