//go:build windows

package install

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// On this system `--shell=bash` is the one selector that can be answered
// wrongly and look right. A stock machine has a bash.exe on PATH belonging to
// the launcher for another operating system, and wiring that one writes into a
// filesystem no session of this machine reads — the install reports success and
// nothing ever runs.
//
// What is asserted is the positive evidence rather than the absence of the
// wrong answer: the interpreter found belongs to a POSIX-emulating environment,
// which is what ships a path translator beside its shell.
func TestTheBashThatIsFoundBelongsToAPosixEnvironment(t *testing.T) {
	exe, err := lookInterpreter(t.Context(), Bash)
	require.NoError(t, err, "this machine is expected to have Git for Windows, whose shell is the one meant")

	translator, found := FindCygpath(exe)

	assert.True(t, found, "%s has no path translator beside it, so it is not a shell an install can wire", exe)
	assert.NotEmpty(t, translator.Exe)
}

// The impostor, with nothing else on the machine to distract from it: a
// bash.exe alone on PATH, with no environment around it. Being on PATH under
// that name is not evidence of anything, and answering with it would wire a
// hook into a filesystem this program cannot see.
func TestABashWithNoEnvironmentAroundItIsNotWired(t *testing.T) {
	only := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(only, "bash.exe"), []byte("not really"), 0o755))
	// Git is off PATH too, so the shell that ships with it cannot be found and
	// this is the only candidate left to judge.
	t.Setenv("PATH", only)

	_, err := lookInterpreter(t.Context(), Bash)

	require.Error(t, err, "the only bash here has no path translator beside it")
	assert.Contains(t, err.Error(), "--shell-exe", "which is how somebody names the real one")
}

// Git is asked where it keeps its own programs rather than a directory being
// assumed, because it can be installed anywhere. The derivation is three levels
// up from that answer, and getting it wrong yields a directory that exists —
// so the candidates are checked for being the shell, not merely for being paths.
func TestGitsOwnShellIsLookedForBeforeTheBareName(t *testing.T) {
	candidates := interpreterCandidates(t.Context(), Bash)

	require.NotEmpty(t, candidates)
	assert.Equal(t, "bash.exe", candidates[len(candidates)-1],
		"the bare name is the last resort, for the environments that do put one on PATH")
	assert.Greater(t, len(candidates), 1, "and Git's own shell is looked for first")
}
