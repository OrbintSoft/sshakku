//go:build windows

package sshtools

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/paths"
)

// aSessionWhosePathIsOnly points this session's lookups at dir and nothing
// else, which is as close as a test gets to being started from a shell that put
// its own tools first.
func aSessionWhosePathIsOnly(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("PATH", dir)
}

// TestTheBuildAShellShipsWithItselfIsNeverTheOneHandedAPassphrase drives the
// real lookup against a directory shaped like the one Git for Windows puts
// first on a Git Bash's PATH: an ssh-add with the MSYS runtime beside it.
//
// Both outcomes are right and both are asserted, because which one a machine
// gives is the machine's business: where this system's own OpenSSH is
// installed, that build is the answer; where it is not, there is no answer and
// the caller is told. What must never happen is the emulated build coming back,
// since running it is what turns a correct passphrase into a key given up on.
func TestTheBuildAShellShipsWithItselfIsNeverTheOneHandedAPassphrase(t *testing.T) {
	itsOwnBin := t.TempDir()
	for _, name := range []string{"ssh-add.exe", msys2Runtime} {
		require.NoError(t, os.WriteFile(filepath.Join(itsOwnBin, name), nil, 0o600), name)
	}
	aSessionWhosePathIsOnly(t, itsOwnBin)

	tool, err := ThisSystem().Tool(SSHAddName)

	assert.NotEqual(t, filepath.Join(itsOwnBin, "ssh-add.exe"), tool,
		"this build cannot open a named pipe, and this system's agent is one")
	if err != nil {
		assert.Contains(t, err.Error(), msys2Runtime, "the refusal has to say what it was refusing")
		return
	}
	assert.True(t, paths.Absent(filepath.Join(filepath.Dir(tool), msys2Runtime)),
		"the program chosen instead is itself beside an emulation runtime")
}

// TestThisSystemNamesWhereItKeepsItsOwnOpenSSH. The directory is read from the
// environment rather than written out, because a Windows installation is not
// always on C:, and a hard-coded drive letter is a lookup that quietly finds
// nothing on the machines it is wrong about.
func TestThisSystemNamesWhereItKeepsItsOwnOpenSSH(t *testing.T) {
	t.Setenv("SystemRoot", filepath.Join("Q:", "NotWindows"))

	assert.Equal(t, []string{filepath.Join("Q:", "NotWindows", "System32", "OpenSSH")},
		ThisSystem().NativeDirs)
}

// TestASystemThatWillNotSayWhereItIsInstalledNamesNoDirectory, rather than
// naming one relative to wherever the process happens to be running from.
func TestASystemThatWillNotSayWhereItIsInstalledNamesNoDirectory(t *testing.T) {
	t.Setenv("SystemRoot", "")

	assert.Empty(t, ThisSystem().NativeDirs)
}

// TestBothWaysOfEmulatingPosixHereAreNamed. Git for Windows brings the first
// and a Cygwin installation the second; a build linked against either one
// reaches this system's agent through nothing at all.
func TestBothWaysOfEmulatingPosixHereAreNamed(t *testing.T) {
	assert.Equal(t, []string{msys2Runtime, cygwinRuntime}, ThisSystem().EmulationRuntimes)
}
