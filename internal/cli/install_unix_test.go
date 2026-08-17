//go:build unix

package cli

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

// aWiredShell is an interpreter this machine really has, and the name the
// command knows it by.
func aWiredShell(t *testing.T) (string, string) {
	t.Helper()
	path, err := exec.LookPath("bash")
	require.NoError(t, err, "this system is expected to have a bash to wire")
	return path, "bash"
}

// installInto points this system's install locations at a directory of the
// test's own, so nothing lands where a real install would put it.
func installInto(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
}
