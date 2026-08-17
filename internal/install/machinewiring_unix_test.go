//go:build unix

package install

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Who may read a machine-wide wiring is a question only a system with real
// permission bits can be asked, which is why it lives here rather than beside
// the rest of the machine-scope tests.
//
// Both of these are read at every account's login. A drop-in directory this
// install created that only its owner could read would take the whole
// directory's worth of wiring with it, silently, and the file inside it is run
// rather than sourced by some of the shells that read such a directory.
func TestAMachineWideDropInAndItsDirectoryAreReadableByEveryLogin(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "profile.d")
	p := bournePlan(t)
	require.NoError(t, p.forMachine(t.Context(), machineWiring{DropInDir: dir}))

	require.NoError(t, p.writeDropIn(". \"/hook.sh\""))

	written, err := os.Stat(p.placement.Path)
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o755), written.Mode().Perm())
	made, err := os.Stat(dir)
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o755), made.Mode().Perm())
}
