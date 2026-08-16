//go:build windows

package cli

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// aWiredShell is an interpreter this machine really has, and the name the
// command knows it by. Windows PowerShell is on every Windows there is, which
// is what makes it the one to ask for.
func aWiredShell(t *testing.T) (string, string) {
	t.Helper()
	path, err := exec.LookPath("powershell.exe")
	require.NoError(t, err, "every Windows has one of these")
	return path, "windowspowershell"
}

// installInto points this system's install locations at a directory of the
// test's own, so nothing lands where a real install would put it.
func installInto(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("LOCALAPPDATA", dir)
}

// F45: one binary on one machine wires a PowerShell and a Git Bash, and neither
// is handed the other's file.
//
// The discriminating question is asked of a real bash: the hook written for it
// is one that bash reads without complaint, and the hook written for PowerShell
// is one it does not. Were the two ever swapped, both halves would flip.
func TestOneMachineWiresAPowerShellAndAGitBashWithoutSwappingTheirFiles(t *testing.T) {
	installInto(t, t.TempDir())

	code, powerShellOut, powerShellErr := wired(t,
		"install", "--shell=windowspowershell", "--profile", aStartupFile(t), "--no-path")
	require.Equalf(t, 0, code, "install: %s%s", powerShellOut, powerShellErr)

	code, bourneOut, bourneErr := wired(t,
		"install", "--shell=bash", "--profile", aStartupFile(t), "--no-path")
	require.Equalf(t, 0, code,
		"install: %s%s (this machine is expected to have Git for Windows)", bourneOut, bourneErr)

	powerShellHook := reported(t, powerShellOut)["hook"]
	bourneHook := reported(t, bourneOut)["hook"]
	require.NotEmpty(t, powerShellHook)
	require.NotEmpty(t, bourneHook)
	require.NotEqual(t, powerShellHook, bourneHook, "each shell gets a hook of its own")

	bash := reported(t, bourneOut)["interpreter"]
	require.NotEmpty(t, bash)
	assert.NoError(t, bashReads(t, bash, bourneHook),
		"the hook written for bash must be one bash reads")
	assert.Error(t, bashReads(t, bash, powerShellHook),
		"and the hook written for PowerShell must not be: it is a different language")
}

// bashReads asks a real bash to read a file without running it, which is how
// this test finds out what language the file is in.
func bashReads(t *testing.T, bash, file string) error {
	t.Helper()
	return exec.CommandContext(t.Context(), bash, "-n", file).Run()
}
