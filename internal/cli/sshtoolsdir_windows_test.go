//go:build windows

package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/cli/shell"
)

// dialectNamed is the language to print for, for a test that knows which it
// wants.
func dialectNamed(t *testing.T, name string) shell.Dialect {
	t.Helper()

	dialect, err := shell.Named(name)
	require.NoError(t, err, name)
	return dialect
}

// aSessionRunning points this session's lookups at dir alone, which is as close
// as a test gets to having been started from a shell that puts its own tools
// first.
func aSessionRunning(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("PATH", dir)
}

// theBinOfARealPosixEnvironment is the directory Git for Windows keeps its own
// OpenSSH in — a real ssh built for MSYS, the MSYS runtime beside it, and that
// environment's own path translator. It is the real thing rather than files
// shaped like it, because what is being asked here is whether a session of that
// kind is given a directory it can actually use.
func theBinOfARealPosixEnvironment(t *testing.T) string {
	t.Helper()

	out, err := exec.CommandContext(t.Context(), "git", "--exec-path").Output()
	if err != nil {
		t.Skip("no git here, so no POSIX-emulating environment to be a session of")
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(strings.TrimSpace(string(out)))))
	bin := filepath.Join(root, "usr", "bin")
	for _, name := range []string{"ssh.exe", "msys-2.0.dll", "cygpath.exe"} {
		if _, err := os.Stat(filepath.Join(bin, name)); err != nil {
			t.Skipf("%s is not the environment this is about: no %s", bin, name)
		}
	}
	return bin
}

// aDirectoryHolding makes a directory containing the named files and returns it.
func aDirectoryHolding(t *testing.T, names ...string) string {
	t.Helper()

	dir := t.TempDir()
	for _, name := range names {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), nil, 0o600), name)
	}
	return dir
}

// TestASessionAlreadyRunningToolsThatReachTheAgentIsAskedToChangeNothing.
// Putting a directory ahead of somebody's own PATH lasts their whole login and
// changes which ssh, scp and sftp they run; it is done for a session that
// cannot otherwise reach its agent, and for no other.
func TestASessionAlreadyRunningToolsThatReachTheAgentIsAskedToChangeNothing(t *testing.T) {
	aSessionRunning(t, aDirectoryHolding(t, "ssh.exe"))

	dir, err := sessionSSHToolsDir(t.Context(), dialectNamed(t, shell.Posix))

	require.NoError(t, err)
	assert.Empty(t, dir)
}

// TestAMachineWithNothingThatCanReachItsOwnAgentSaysSo. The session is left
// exactly as it was — there is nothing better to point it at — and the reason
// is something a person can read, rather than a shell that goes on quietly
// asking for a passphrase at every push.
func TestAMachineWithNothingThatCanReachItsOwnAgentSaysSo(t *testing.T) {
	aSessionRunning(t, aDirectoryHolding(t, "ssh.exe", "msys-2.0.dll"))
	t.Setenv("SystemRoot", filepath.Join(t.TempDir(), "no-windows-was-installed-here"))

	dir, err := sessionSSHToolsDir(t.Context(), dialectNamed(t, shell.Posix))

	require.ErrorIs(t, err, errNoBuildForThisSession)
	assert.Empty(t, dir)
}

// TestASessionThatShipsNoTranslatorIsSaidSoRatherThanGuessedFor. Which spelling
// such an environment reads is its own business and it ships the answer as a
// program; one that has no such program is not one to invent a mapping for,
// since a directory spelled wrong goes on the PATH and reaches nothing.
func TestASessionThatShipsNoTranslatorIsSaidSoRatherThanGuessedFor(t *testing.T) {
	aSessionRunning(t, aDirectoryHolding(t, "ssh.exe", "msys-2.0.dll"))

	dir, err := sessionSSHToolsDir(t.Context(), dialectNamed(t, shell.Posix))

	require.ErrorIs(t, err, errNoTranslatorForThisSession)
	assert.Empty(t, dir)
}

// TestARealPosixSessionIsHandedADirectoryItCanRead drives the whole of it: a
// session of a real POSIX-emulating environment, the real lookup, this system's
// real OpenSSH, and that environment's real translator. A directory in this
// system's own spelling would go on that shell's PATH and name nothing.
func TestARealPosixSessionIsHandedADirectoryItCanRead(t *testing.T) {
	aSessionRunning(t, theBinOfARealPosixEnvironment(t))

	dir, err := sessionSSHToolsDir(t.Context(), dialectNamed(t, shell.Posix))

	require.NoError(t, err, "this system keeps an OpenSSH of its own, and that session ships a translator")
	assert.True(t, strings.HasPrefix(dir, "/"), "a path such a shell reads starts at its own root: %q", dir)
	assert.NotContains(t, dir, `\`, "a backslash names nothing that shell can open")
	assert.Contains(t, strings.ToLower(dir), "openssh", "and it is where the tools are")
}

// TestASessionOfThisSystemsOwnKindIsHandedThisSystemsSpelling. The same
// directory, in the writing a PowerShell reads — nothing is translated for a
// session that already spells paths the way this program does.
func TestASessionOfThisSystemsOwnKindIsHandedThisSystemsSpelling(t *testing.T) {
	aSessionRunning(t, theBinOfARealPosixEnvironment(t))

	dir, err := sessionSSHToolsDir(t.Context(), dialectNamed(t, shell.PowerShell))

	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(dir, "ssh.exe"), "the directory named must be one holding the tools")
	assert.True(t, filepath.IsAbs(dir), "and it is named absolutely, since a session's own directory is not where it is")
}
