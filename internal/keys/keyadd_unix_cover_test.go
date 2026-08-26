//go:build unix

package keys

import (
	"errors"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The failures these tests hand their seams. Each stands for a real one the
// code under test cannot be made to produce on demand.
var (
	errForkExecPermissionDenied = errors.New("fork/exec: permission denied")
	errNoKeyring                = errors.New("no keyring")
)

// saveKeyaddSeams snapshots the stash and ssh-add-run seams, restoring them
// when the (sub)test ends.
func saveKeyaddSeams(t *testing.T) {
	t.Helper()
	oStash, oRun := stashPass, runCmd
	t.Cleanup(func() { stashPass, runCmd = oStash, oRun })
}

func TestAddWithAskpassStashError(t *testing.T) {
	saveKeyaddSeams(t)
	stashPass = func(string, time.Duration) (string, error) { return "", errNoKeyring }
	a := ExecKeyAdder{AskpassProg: "/usr/bin/sshakku"}
	_, err := a.AddWithAskpass(t.Context(), "/home/u/.ssh/id_rsa", "pw")
	assert.Error(t, err,
		"with nowhere to put the passphrase for the helper, ssh-add would prompt on a terminal nobody is watching")
}

func TestAddWithAskpassRunsSSHAdd(t *testing.T) {
	saveKeyaddSeams(t)
	var stashedTTL time.Duration
	stashPass = func(_ string, ttl time.Duration) (string, error) { stashedTTL = ttl; return "token", nil }
	runCmd = func(*exec.Cmd) error { return nil }
	a := ExecKeyAdder{AskpassProg: "/usr/bin/sshakku"}
	rc, err := a.AddWithAskpass(t.Context(), "/home/u/.ssh/id_rsa", "pw")
	require.NoError(t, err, "running ssh-add must succeed")
	assert.Zero(t, rc, "and a key that opened exits zero")
	assert.Equal(t, defaultKeyTTL, stashedTTL,
		"the passphrase is put aside only for as long as the helper needs to collect it")
}

func TestRunSSHAddExitCode(t *testing.T) {
	saveKeyaddSeams(t)
	// A real non-zero process exit yields the *exec.ExitError runSSHAdd must
	// translate into a returned exit code (a wrong passphrase, not a failure).
	realExit := exec.CommandContext(t.Context(), "sh", "-c", "exit 3").Run()
	if _, ok := errors.AsType[*exec.ExitError](realExit); !ok {
		t.Skipf("could not obtain an ExitError in this environment: %v", realExit)
	}
	runCmd = func(*exec.Cmd) error { return realExit }

	rc, err := (ExecKeyAdder{}).runSSHAdd(t.Context(), nil, "/home/u/.ssh/id_rsa")
	require.NoError(t, err,
		"a wrong passphrase is what ssh-add exiting non-zero means, and the loader retries on it rather than giving up")
	assert.Equal(t, 3, rc, "so the exit code must be handed back as it was")
}

func TestRunSSHAddStartFailure(t *testing.T) {
	saveKeyaddSeams(t)
	runCmd = func(*exec.Cmd) error { return errForkExecPermissionDenied }
	rc, err := (ExecKeyAdder{}).runSSHAdd(t.Context(), nil, "/home/u/.ssh/id_rsa")
	assert.Error(t, err, "ssh-add not running at all is not a wrong passphrase, and retrying would not help")
	assert.Zero(t, rc, "so there is no exit code to report")
}
