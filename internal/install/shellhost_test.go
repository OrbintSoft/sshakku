package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWhatAShellSaidIsReadOffLineByLine(t *testing.T) {
	shell, err := parseShell([]byte("home=/c/Users/example\nshell=/usr/bin/bash\nversion=5.2.37(1)-release\n"))

	require.NoError(t, err)
	assert.Equal(t, "/c/Users/example", shell.Home)
	assert.Equal(t, "/usr/bin/bash", shell.Exe)
	assert.Equal(t, "5.2.37(1)-release", shell.Version)
}

// A home directory with a space in it is ordinary on the platform this is for,
// and the value is everything after the first separator — a version string has
// its own '=' in it often enough to matter.
func TestAValueIsEverythingAfterTheFirstSeparator(t *testing.T) {
	shell, err := parseShell([]byte("home=/c/Users/O'Brien & Co\nversion=x=y=z\n"))

	require.NoError(t, err)
	assert.Equal(t, "/c/Users/O'Brien & Co", shell.Home)
	assert.Equal(t, "x=y=z", shell.Version)
}

// The environment this runs under may end a line either way, and a carriage
// return carried into a path makes a file name nothing can open.
func TestACarriageReturnIsNotPartOfTheAnswer(t *testing.T) {
	shell, err := parseShell([]byte("home=/c/Users/example\r\nshell=/usr/bin/bash\r\n"))

	require.NoError(t, err)
	assert.Equal(t, "/c/Users/example", shell.Home)
	assert.Equal(t, "/usr/bin/bash", shell.Exe)
}

// A shell may answer more than this version asks, and an older reader must not
// refuse a newer script over a line it has no use for.
func TestAKeyNobodyKnowsIsIgnoredRatherThanRefused(t *testing.T) {
	shell, err := parseShell([]byte("home=/h\nsomething-new=yes\nnot a pair at all\n"))

	require.NoError(t, err)
	assert.Equal(t, "/h", shell.Home)
}

func TestAShellThatSaidNothingIsAFailure(t *testing.T) {
	_, err := parseShell(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "printed nothing")

	_, err = parseShell([]byte("  \r\n \n"))
	require.Error(t, err, "whitespace is nothing")
}

// Without a home there is no file to wire. Reading that as a shell whose home
// is the empty string would wire whatever directory the install ran from.
func TestAShellThatNamedNoHomeIsAFailure(t *testing.T) {
	_, err := parseShell([]byte("shell=/usr/bin/bash\nversion=5\n"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "nothing to wire")
}

func TestTheEmbeddedQueryIsTheScriptInTheTree(t *testing.T) {
	require.NotEmpty(t, queryShellScript)
	assert.True(t, strings.HasPrefix(string(queryShellScript), "#!"),
		"the shell decides what to do with a file by its first line")
	assert.Contains(t, string(queryShellScript), "home=")

	path, cleanup, err := shellScriptOnDisk()
	require.NoError(t, err)
	defer cleanup()

	written, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, queryShellScript, written)

	cleanup()
	_, err = os.Stat(path)
	assert.True(t, os.IsNotExist(err), "a query leaves nothing behind on the disk it borrowed")
}

func TestAskingSomethingThatIsNotAShellNamesIt(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such-shell")

	_, err := AskShell(t.Context(), missing)

	require.Error(t, err)
	assert.Contains(t, err.Error(), missing)
}

// The seam this exists for: a real shell, handed the real script, answering
// about itself. Everything above tests what is done with the answer; only this
// tests that the question can be asked at all.
func TestARealShellAnswersAboutItself(t *testing.T) {
	bash := findBash(t, mustAbs(t, hookLib))

	shell, err := AskShell(t.Context(), bash)

	require.NoError(t, err)
	assert.NotEmpty(t, shell.Home, "every shell that runs has a home directory it reads startup files from")
	assert.True(t, strings.HasPrefix(shell.Home, "/"),
		"and it is in the shell's own spelling, not this program's; got %q", shell.Home)
	assert.NotEmpty(t, shell.Version, "a report quotes what the shell calls itself")
}

// The answer has to be about the shell being asked, not about the environment
// this program happens to be running in.
func TestTheShellAnswersForItselfAndNotForUs(t *testing.T) {
	bash := findBash(t, mustAbs(t, hookLib))
	t.Setenv("HOME", "/somewhere/this/program/invented")

	shell, err := AskShell(t.Context(), bash)

	require.NoError(t, err)
	assert.Equal(t, "/somewhere/this/program/invented", shell.Home,
		"a shell inherits the environment it is started with, and reports what it will really use")
}

func mustAbs(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	require.NoError(t, err)
	return abs
}
