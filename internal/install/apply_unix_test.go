//go:build unix

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
	return "/somewhere/bash", Bash
}

// aLiveInterpreter is an interpreter this machine really has, for a test that
// runs one.
func aLiveInterpreter(t *testing.T) (string, ShellKind) {
	t.Helper()
	path, err := exec.LookPath("bash")
	require.NoError(t, err, "this suite needs a bash to ask")
	return path, Bash
}

// installInto points this system's install locations at a directory of the
// test's own, so that nothing is written where a real install would put it.
func installInto(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
}

// dropInDirBeside is where this platform's install would look for a drop-in
// directory beside the startup file it is wiring.
func dropInDirBeside(startupFile string) string {
	return BourneDropInDir(startupFile)
}
