//go:build windows

package install

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

// aShellName is a name this system's table recognises, and the kind it is. The
// file need not exist: what is being exercised is the recognition, which reads
// the name and not the disk.
func aShellName() (string, ShellKind) {
	return `C:\somewhere\powershell.exe`, WindowsPowerShell
}

// aLiveInterpreter is an interpreter this machine really has, for a test that
// runs one. Windows PowerShell is on every Windows there is, which is what
// makes it the one to ask.
func aLiveInterpreter(t *testing.T) (string, ShellKind) {
	t.Helper()
	path, err := exec.LookPath("powershell.exe")
	require.NoError(t, err, "every Windows has one of these")
	return path, WindowsPowerShell
}

// installInto points this system's install locations at a directory of the
// test's own, so that nothing is written where a real install would put it.
func installInto(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("LOCALAPPDATA", dir)
}
