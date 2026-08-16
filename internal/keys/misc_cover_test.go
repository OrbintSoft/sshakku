package keys

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/run"
	"github.com/OrbintSoft/sshakku/internal/testproc"
)

// TestEnumeratorReadDirHardError covers Keys's non-missing-directory error
// branch: pointing the enumerator at a regular file (not a directory) makes
// ReadDir fail with something other than "does not exist", which propagates.
func TestEnumeratorReadDirHardError(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600), "seed a file where a directory should be")
	keys, err := Enumerator{Dir: file}.Keys()
	assert.Error(t, err, "a key directory that is not a directory must be reported, not read as an account with no keys")
	assert.Empty(t, keys, "and nothing may be offered to ssh-add")
}

// TestExecRunnerRunStdinEnvAndStartFailure covers Run's stdin- and env-passing
// branches (via a real process that echoes its stdin — this test binary,
// re-entered; see internal/testproc) and its start-failure branch (a binary
// that does not exist, which is a real error rather than a non-zero exit code).
func TestExecRunnerRunStdinEnvAndStartFailure(t *testing.T) {
	name, args := testproc.Command(t, testproc.EchoStdin)
	res, err := run.ExecRunner{}.Run(t.Context(), run.Cmd{
		Name: name, Args: args, Stdin: "hello", Env: []string{"SSHAKKU_X=1"},
	})
	require.NoError(t, err, "running a command must succeed")
	assert.Equal(t, "hello", string(res.Stdout),
		"and what it was given on standard input must reach it: that is where every secret travels")

	_, err = run.ExecRunner{}.Run(t.Context(), run.Cmd{Name: "sshakku-no-such-binary-xyz"})
	assert.Error(t, err, "a program that is not installed must be reported as that, not as one that ran and failed")
}
