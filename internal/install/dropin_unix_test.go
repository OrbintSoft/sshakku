//go:build unix

package install

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// F19, F44: where a Bourne shell reads a drop-in directory beside its startup
// file, a file of ours goes in there and the startup file is not touched at all —
// and the uninstall takes that file away again rather than looking for a block
// inside it.
//
// The rule under test is that shell's: a directory that exists is one somebody's
// own configuration loops over, and its existence is the whole condition. It is
// asked of this system's own shell, which is a Bourne one. PowerShell's rule is
// the opposite — nothing but a profile can load such a directory, so the profile
// is read first — and it is asked where a PowerShell is, with the same promise
// behind it (internal/cli's `--shell=bash` case covers this one there).
func TestWhereAShellReadsADropInDirectoryTheWiringIsAFileOfItsOwn(t *testing.T) {
	home := t.TempDir()
	installInto(t, home)
	profile := filepath.Join(home, "startup-file")
	require.NoError(t, os.WriteFile(profile, []byte("# mine\n"), 0o644))
	require.NoError(t, os.Mkdir(dropInDirBeside(profile), 0o755))
	req := wiringRequest(t, home, profile)

	installed, err := Install(t.Context(), req, Ancestry{})
	require.NoError(t, err)

	assert.True(t, installed.DropIn, "a directory that is there is one somebody's own configuration loops over")
	assert.FileExists(t, installed.Wired)
	untouched, err := os.ReadFile(profile)
	require.NoError(t, err)
	assert.Equal(t, "# mine\n", string(untouched), "the shell's own file is left exactly as it was")

	_, err = Uninstall(t.Context(), req, Ancestry{})
	require.NoError(t, err)
	assert.NoFileExists(t, installed.Wired)
	after, err := os.ReadFile(profile)
	require.NoError(t, err)
	assert.Equal(t, "# mine\n", string(after))
}
