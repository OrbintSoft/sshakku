//go:build windows

package install

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/diagnose/launcher"
)

// A process table here gives a file name with no directory, which is exactly
// what cannot tell two programs of the same name apart. This asks the system
// for the whole path, about the one process whose answer is already known.
func TestTheRealSystemGivesTheWholePathOfARunningProgram(t *testing.T) {
	mine, ok := ImagePath(os.Getpid())

	require.True(t, ok, "a running process must be able to learn what it is running")
	assert.True(t, filepath.IsAbs(mine), "a name without a directory is what this improves on; got %q", mine)

	expected, err := os.Executable()
	require.NoError(t, err)
	assert.Equal(t, strings.ToLower(expected), strings.ToLower(mine))
}

func TestAProcessThatIsNotThereHasNoImage(t *testing.T) {
	// Above the range this system hands pids out in, so nothing can be running
	// under it and a false answer cannot be a coincidence.
	_, ok := ImagePath(1 << 30)

	assert.False(t, ok, "a process that is not there must not be answered for")
}

// The measurement this confirmation exists for. Two programs on this machine
// are called bash.exe: the shell shipped with Git for Windows, and the launcher
// for another operating system, in the system directory. Wiring a hook into the
// second would write into a filesystem this program cannot see, for sessions it
// cannot start — and a process table names them identically.
func TestTheTwoProgramsCalledBashAreToldApart(t *testing.T) {
	shell := gitBash(t)

	kind, ok := RecogniseShell(shell)
	require.True(t, ok, "the shell of a POSIX-emulating environment is one this can wire")
	assert.Equal(t, Bash, kind)

	root, ok := os.LookupEnv("SystemRoot")
	if !ok {
		t.Skip("this system does not say where it is installed")
	}
	elsewhere := filepath.Join(root, "System32", filepath.Base(shell))
	if _, err := os.Stat(elsewhere); err != nil {
		t.Skip("no second program of that name is installed here")
	}

	_, ok = RecogniseShell(elsewhere)

	assert.False(t, ok,
		"%s answers to the same name and is no shell an install can wire; it must not be guessed at", elsewhere)
}

// The editions are separate targets, and these are the images the system really
// runs them under.
func TestTheRealPowerShellsAreRecognisedByTheirOwnImages(t *testing.T) {
	for _, c := range []struct {
		exe  string
		want ShellKind
	}{
		{"pwsh", PowerShellCore},
		{"powershell", WindowsPowerShell},
	} {
		path, err := exec.LookPath(c.exe)
		if err != nil {
			t.Logf("no %s here to recognise", c.exe)
			continue
		}

		kind, ok := RecogniseShell(path)

		require.True(t, ok, "%s at %s", c.exe, path)
		assert.Equal(t, c.want, kind)
	}
}

// End to end on the real machine: the real process tree, the real image paths,
// and whatever this test binary was really started by. It asserts an outcome
// rather than a particular answer — which shell is above a test run depends on
// how the run was started — but either outcome has to be a truthful one.
func TestResolvingAgainstTheRealMachineEitherNamesAShellOrAsksToBeTold(t *testing.T) {
	kind, path, err := ResolveShell(t.Context(), os.Getpid(), launcher.NewToolhelpAncestry(), ImagePath)
	if err != nil {
		assert.Contains(t, err.Error(), "--shell", "not knowing has to be said, never guessed past")
		return
	}
	assert.Contains(t, offeredShellKinds(), kind)
	assert.FileExists(t, path, "the interpreter named is one that is really there")
	again, ok := RecogniseShell(path)
	require.True(t, ok)
	assert.Equal(t, kind, again, "and asking about the path it returned gives the same answer")
}
