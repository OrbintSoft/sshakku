package install

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// asASessionThatMayChangeTheMachine and its opposite hand the install this
// system's answer about the session running it.
//
// The answer is handed in rather than arranged for, because a suite runs as
// whoever started it and neither answer can be produced on demand: a session
// with the authority cannot drop it, and one without it cannot take it. What
// the install *does* with each answer is the code's own decision and stays
// where it is.
func asASessionThatMayChangeTheMachine(t *testing.T, may bool) {
	t.Helper()

	previous := runningForTheMachine
	runningForTheMachine = func() bool { return may }
	t.Cleanup(func() { runningForTheMachine = previous })
}

// machineRequest is an install for the whole machine, wiring the file named and
// leaving the stored environment alone.
func machineRequest(t *testing.T, home, profile string) Request {
	t.Helper()

	req := wiringRequest(t, home, profile)
	req.Scope, req.NoPath = Machine, true
	return req
}

// F59: a machine-wide install that is not being run with the authority to
// finish one stops before creating anything, rather than writing the file every
// login runs into a directory an ordinary account owns and can replace.
func TestAMachineWideInstallWithoutTheAuthorityWritesNothing(t *testing.T) {
	home := t.TempDir()
	installInto(t, home)
	asASessionThatMayChangeTheMachine(t, false)
	profile := filepath.Join(home, "startup-file")

	_, err := Install(t.Context(), machineRequest(t, home, profile), Ancestry{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), machineAuthorityName,
		"the refusal names the authority this system asks such a change to be made with")
	assert.NoFileExists(t, profile, "and no file is written for a change that was refused")
}

// F59: the same authority is asked for before taking a machine-wide wiring out,
// which is a change to what every account runs just as putting it in was. A
// session that cannot finish one leaves the machine wired rather than
// half-unwired.
func TestAMachineWideUninstallWithoutTheAuthorityLeavesTheWiringAlone(t *testing.T) {
	home := t.TempDir()
	installInto(t, home)
	asASessionThatMayChangeTheMachine(t, false)
	profile := filepath.Join(home, "startup-file")
	// The wiring is put there as an earlier install left it, rather than by
	// running one: a machine-wide install writes where the machine keeps such
	// things, and this test is about what happens to a file, not about being
	// allowed to write in that directory.
	require.NoError(t, UpsertBlockFile(profile, ". '"+filepath.Join(home, "shell-hook")+"'"))
	before, err := os.ReadFile(profile)
	require.NoError(t, err)

	_, err = Uninstall(t.Context(), machineRequest(t, home, profile), Ancestry{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), machineAuthorityName)
	after, err := os.ReadFile(profile)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "the wiring is left exactly as it was")
}

// F59: what needs no authority is not refused for want of it. An install for
// the account running the command writes only where that account already may,
// so the question never arises.
func TestAnInstallForThisAccountAloneNeedsNoAuthority(t *testing.T) {
	home := t.TempDir()
	installInto(t, home)
	asASessionThatMayChangeTheMachine(t, false)
	profile := filepath.Join(home, "startup-file")
	req := wiringRequest(t, home, profile)
	req.NoPath = true

	out, err := Install(t.Context(), req, Ancestry{})

	require.NoError(t, err)
	assert.FileExists(t, out.HookFile, "the hook is written for an account's own install")
	assert.FileExists(t, profile, "and the file that runs it is wired")
}
