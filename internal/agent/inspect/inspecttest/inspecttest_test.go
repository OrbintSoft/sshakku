package inspecttest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The tree written here is what every test of the process scan reads its
// answers from, so a helper that writes the kernel's format only approximately
// does not fail — it makes the tests above it agree with a /proc no machine
// ever produces. That is what these tests are for.

func TestFakeProcWritesTheKernelsCmdlineFormat(t *testing.T) {
	root := t.TempDir()
	FakeProc(t, root, 100, []string{"ssh-agent", "-a", "/run/agent.sock"}, 1000)

	cmdline, err := os.ReadFile(filepath.Join(root, "100", "cmdline"))
	require.NoError(t, err, "the pid directory must hold a cmdline")
	assert.Equal(t, "ssh-agent\x00-a\x00/run/agent.sock\x00", string(cmdline),
		"the kernel separates arguments with NUL and terminates the last one too; a reader that "+
			"trims trailing NULs would pass either way, and one that splits on them would gain an empty argument")
}

func TestFakeProcWritesAnEmptyCmdlineForNoArgv(t *testing.T) {
	root := t.TempDir()
	FakeProc(t, root, 600, nil, 1000)

	cmdline, err := os.ReadFile(filepath.Join(root, "600", "cmdline"))
	require.NoError(t, err, "a kernel thread still has a cmdline file")
	assert.Empty(t, cmdline,
		"a kernel thread's cmdline is empty, not a lone NUL — which would read back as one empty argument")
}

func TestFakeProcWritesTheOwnerInTheStatusFile(t *testing.T) {
	root := t.TempDir()
	FakeProc(t, root, 200, []string{"ssh-agent"}, 1000)

	status, err := os.ReadFile(filepath.Join(root, "200", "status"))
	require.NoError(t, err, "a process with a known owner must have a status file")
	assert.Contains(t, string(status), "Uid:\t1000\t1000\t1000\t1000\n",
		"the owner is read from the Uid line's first field, and the kernel writes four of them tab-separated")
}

func TestFakeProcOmitsTheStatusFileForAnUnknownOwner(t *testing.T) {
	root := t.TempDir()
	FakeProc(t, root, 700, []string{"ssh-agent"}, -1)

	_, err := os.Stat(filepath.Join(root, "700", "status"))
	assert.ErrorIs(t, err, os.ErrNotExist,
		"a negative uid asks for a process whose owner cannot be read; writing any status file at all "+
			"would give the scan an owner to find and the unknown-owner case would never be exercised")
}
