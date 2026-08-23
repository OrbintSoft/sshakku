//go:build windows

package keys

import (
	"context"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// exitOfSSHAdd starts this system's own ssh-add with exactly env and reports
// what it exited with. `-l` asks it only to list, so it needs no key, no
// passphrase and no wallet — what is being measured is whether the program can
// start at all with what it was given.
func exitOfSSHAdd(ctx context.Context, t *testing.T, env []string) int {
	t.Helper()

	cmd := exec.CommandContext(ctx, "ssh-add", "-l")
	cmd.Env = env
	err := cmd.Run()
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr, "ssh-add could not be run at all")
	return exitErr.ExitCode()
}

// startupFailure is what this system's ssh-add exits with when it cannot get
// going: no message on either stream, and this code.
const startupFailure = 255

// TestSSHAddCanStartWithWhatThisSystemSaysToGiveIt is the whole point of the
// per-system list. ssh-add is handed an environment built up from nothing
// rather than inherited, and a program on this system needs more of one than a
// program on the others does.
//
// It asks for a listing, which needs no key and no agent, so what it answers is
// only whether it started. Anything but the startup failure means it did.
func TestSSHAddCanStartWithWhatThisSystemSaysToGiveIt(t *testing.T) {
	_, err := exec.LookPath("ssh-add")
	require.NoError(t, err, "this platform's own ssh-add is what SSHakku drives here; without it nothing below is answerable")

	code := exitOfSSHAdd(t.Context(), t, passThrough(nil, childEnvNames(platformChildEnv)...))

	assert.NotEqualf(t, startupFailure, code,
		"ssh-add could not start with the environment this system's list gives it, which is how a stored passphrase comes to be reported as stale")
}

// TestTheDirectoryTheSystemKeepsSSHsOwnConfigurationUnderIsWhyProgramDataIsInThatList
// records the measurement the list rests on, so that if this system stops
// needing it, somebody is told rather than left carrying an entry nobody can
// account for.
//
// Dropping it is not a degradation, it is a stop: ssh-add exits with the code
// above having printed nothing at all, on either stream. Nothing about that
// points at an environment, which is why it had to be measured rather than
// reasoned about — and why what a user would otherwise see is a session log
// telling them their perfectly good stored passphrase has gone stale.
func TestTheDirectoryTheSystemKeepsSSHsOwnConfigurationUnderIsWhyProgramDataIsInThatList(t *testing.T) {
	require.Contains(t, platformChildEnv, "ProgramData")

	var without []string
	for _, name := range childEnvNames(platformChildEnv) {
		if name != "ProgramData" {
			without = append(without, name)
		}
	}

	code := exitOfSSHAdd(t.Context(), t, passThrough(nil, without...))

	assert.Equalf(t, startupFailure, code,
		"ssh-add started without ProgramData, so the reason that entry is in the list no longer holds: measure again before trusting either answer")
}
