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
	"golang.org/x/sys/windows"
)

// bashCandidates says where this system keeps a bash that can run the shell
// library, in the order they should be tried.
//
// The name "bash" is deliberately absent. On Windows it resolves to
// C:\Windows\System32\bash.exe, which launches WSL: a real bash, but one whose
// filesystem is a different one, where this repository either is not present or
// is present under another path. Comparing against it would compare against the
// wrong machine.
//
// The bash that belongs to this machine is the one shipped with Git for
// Windows, and where Git is installed is asked of Git rather than assumed —
// `git --exec-path` reports a directory three levels below the installation
// root, so the root is what encloses it.
func bashCandidates(t *testing.T) []string {
	t.Helper()

	out, err := exec.CommandContext(t.Context(), "git", "--exec-path").Output()
	if err != nil {
		return nil
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(strings.TrimSpace(string(out)))))
	return []string{
		filepath.Join(root, "bin", "bash.exe"),
		filepath.Join(root, "usr", "bin", "bash.exe"),
	}
}

// heldOpenBySomethingElse holds path the way a program using a file holds it:
// anything may read it, and nothing may delete it while the handle is open.
//
// This is what a profile open in an editor is, and it is the difference this
// system makes that the others do not — elsewhere a file can be unlinked while
// somebody has it open, and here it cannot. An uninstall meets that, so it has
// to say so.
func heldOpenBySomethingElse(t *testing.T, path string) {
	t.Helper()

	wide, err := windows.UTF16PtrFromString(path)
	require.NoError(t, err)
	handle, err := windows.CreateFile(wide, windows.GENERIC_READ, windows.FILE_SHARE_READ,
		nil, windows.OPEN_EXISTING, 0, 0)
	require.NoError(t, err)
	t.Cleanup(func() { _ = windows.CloseHandle(handle) })
}

// F44: a startup file that held nothing but the wiring is taken away with it,
// and one that cannot be taken away is reported — not passed over as swept. The
// file is still there afterwards, which is exactly why the report matters:
// something is left to be dealt with and the person has to hear it now.
func TestAStartupFileHeldOpenElsewhereIsReportedRatherThanSweptSilently(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Microsoft.PowerShell_profile.ps1")
	require.NoError(t, UpsertBlockFile(path, ". 'C:\\hook.ps1'"))
	heldOpenBySomethingElse(t, path)

	err := StripBlockFile(path)

	require.Error(t, err)
	assert.Contains(t, err.Error(), path, "which file was not removed is what to say")
	assert.FileExists(t, path, "and it is still there, which is what the refusal is about")
}

// F44: and the same for the hook itself where it was written as a drop-in.
func TestADropInHookHeldOpenElsewhereIsReported(t *testing.T) {
	path := filepath.Join(t.TempDir(), "50-sshakku.ps1")
	require.NoError(t, os.WriteFile(path, []byte("# the hook\n"), 0o600))
	heldOpenBySomethingElse(t, path)

	err := RemoveDropIn(path)

	require.Error(t, err)
	assert.Contains(t, err.Error(), path)
	assert.FileExists(t, path)
}
