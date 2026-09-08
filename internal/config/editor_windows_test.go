//go:build windows

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows/registry"
)

// appPathsKey is where this system records what a program of a given name is,
// for a name that is not on PATH.
const appPathsKey = `SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\`

// TestAProgramOnThePathIsFound covers the ordinary half of the looking, with a
// program every Windows has.
func TestAProgramOnThePathIsFound(t *testing.T) {
	path, found := findEditor("cmd.exe")
	require.True(t, found, "a program on PATH must be found")
	assert.FileExists(t, path, "and what comes back must be where it is")
}

// TestAProgramRegisteredWhereItWasInstalledIsFound covers the half PATH does
// not answer, and the reason this system's looking is not exec.LookPath alone:
// a program that installs itself outside PATH records where it went under App
// Paths, which is what the system's own Run box resolves a bare name through.
// Notepad++ is one such, and a search of PATH would miss it on every machine
// that has it.
//
// The entry is made under this account rather than the machine's, so nothing
// outside the account running the test is touched, and it is removed
// afterwards whether the test passes or not.
func TestAProgramRegisteredWhereItWasInstalledIsFound(t *testing.T) {
	// A path with a space in it, which is where this system installs programs.
	dir := filepath.Join(t.TempDir(), "Editor Programs")
	require.NoError(t, os.MkdirAll(dir, 0o700), "make a directory with a space in its name")
	program := filepath.Join(dir, "sshakku-test-editor.exe")
	require.NoError(t, os.WriteFile(program, nil, 0o600), "put something where the entry will point")

	registerProgram(t, "sshakku-test-editor.exe", program)

	path, found := findEditor("sshakku-test-editor.exe")
	require.True(t, found, "a program this system says is installed must be found")
	assert.Equal(t, program, path, "and what comes back must be where this system says it is")
}

// TestAProgramRegisteredWhereNothingIsLeftIsNotFound covers the entry that
// outlived its program: reporting it would have SSHakku run a path to nothing
// and fail, where passing over it leaves the next editor to be tried.
func TestAProgramRegisteredWhereNothingIsLeftIsNotFound(t *testing.T) {
	gone := filepath.Join(t.TempDir(), "uninstalled", "sshakku-test-editor.exe")
	registerProgram(t, "sshakku-test-editor.exe", gone)

	_, found := findEditor("sshakku-test-editor.exe")
	assert.False(t, found, "an entry naming a program that is no longer there is not an editor")
}

// TestOneOfTheEditorsThisSystemFallsBackOnIsHere is the claim this platform can
// make and no other can: whatever a particular Windows carries, it carries one
// of these, so `--edit` on an account that named no editor opens something.
func TestOneOfTheEditorsThisSystemFallsBackOnIsHere(t *testing.T) {
	found := firstEditorFound(platformEditors, findEditor)
	assert.FileExists(t, found,
		"with nothing named anywhere SSHakku would open %q, which this system has no way to run", found)
}

// registerProgram tells this system where a program of that name is, the way an
// installer does, and takes the entry away again when the test ends.
func registerProgram(t *testing.T, name, path string) {
	t.Helper()

	key, _, err := registry.CreateKey(registry.CURRENT_USER, appPathsKey+name, registry.WRITE)
	require.NoError(t, err, "record where a program is installed")
	t.Cleanup(func() {
		_ = registry.DeleteKey(registry.CURRENT_USER, appPathsKey+name)
	})
	require.NoError(t, key.SetStringValue("", path), "write the path the entry names")
	require.NoError(t, key.Close(), "close the entry")
}
