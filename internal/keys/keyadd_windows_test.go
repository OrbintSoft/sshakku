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
//
// What makes it fail depends on the machine, and that is deliberate rather
// than a weakness: on a machine where the system-wide ssh configuration
// directory exists, an environment missing the variable that names it is fatal
// and this catches it; on one where that directory was never created, the same
// environment is survivable and this passes. The assertion is on the outcome
// that matters everywhere — the program SSHakku is about to hand a passphrase
// to can start — rather than on a mechanism that only some machines have.
func TestSSHAddCanStartWithWhatThisSystemSaysToGiveIt(t *testing.T) {
	_, err := exec.LookPath("ssh-add")
	require.NoError(t, err, "this platform's own ssh-add is what SSHakku drives here; without it nothing below is answerable")

	code := exitOfSSHAdd(t.Context(), t, passThrough(nil, childEnvNames(platformChildEnv)...))

	assert.NotEqualf(t, startupFailure, code,
		"ssh-add could not start with the environment this system's list gives it, which is how a stored passphrase comes to be reported as stale")
}

// TestTheSystemWideConfigurationDirectoryIsNamedInThatList keeps the entry that
// cost a measurement from being trimmed by someone reading the list and seeing
// nothing that obviously belongs to SSHakku.
//
// It says only that the name is there, and that is as much as can honestly be
// asserted: what happens without it is the machine's business, not this
// platform's. Where the system-wide ssh configuration directory exists,
// ssh-add given no way to find it exits 255 having printed nothing at all, on
// either stream — the failure this entry was added for, and the one the test
// above catches. Where that directory was never created, the same environment
// costs nothing, which is why a test asserting the failure passed on the
// machine it was written on and failed on the runner. The entry is right in
// both cases: a child that cannot find the machine's ssh configuration is
// reading a different one from the session that started it, whether or not it
// gets far enough to say so.
func TestTheSystemWideConfigurationDirectoryIsNamedInThatList(t *testing.T) {
	assert.Contains(t, platformChildEnv, "ProgramData",
		"without it, ssh-add cannot find the configuration this system keeps for every account")
}
