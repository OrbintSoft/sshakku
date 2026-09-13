//go:build windows

package install

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

// F59: the authority a machine-wide install needs is this session's to have or
// not, and what this system says about it has to be what this system will
// actually allow — an install that asked the wrong question would refuse a
// session that could have finished, or write half of one that could not.
//
// The two answers are arrived at differently on purpose. This one asks the
// process token; the test asks the service manager for the right to add a
// service to the machine, which is the same authority reached through a door
// this code never uses. Asking the token twice would be the implementation
// agreeing with itself, and would pass however wrong it was.
//
// Both answers can be had from either kind of session: an ordinary one says no
// twice and an administrator's says yes twice, so this is a real check wherever
// it runs rather than one that only means something on a machine nobody has.
// Nothing is created — the handle is asked for and given straight back.
func TestTheAuthorityThisSessionClaimsIsTheOneTheMachineGrantsIt(t *testing.T) {
	manager, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CREATE_SERVICE)
	mayChangeTheMachine := err == nil
	if mayChangeTheMachine {
		require.NoError(t, windows.CloseServiceHandle(manager))
	}

	assert.Equal(t, mayChangeTheMachine, haveMachineAuthority(),
		"what this session says it may do to the machine, and what the machine lets it do, are one answer")
}

// sharedDirectoryOfItsOwn points a machine-wide install at a directory of the
// test's own and returns where the hook would go inside it.
//
// This is the platform's own answer and that is why these two tests are here:
// where the machine keeps what every account reads is a variable on this
// system, and a fixed path on the others — which cannot be pointed anywhere
// and so cannot be written into by a test at all.
func sharedDirectoryOfItsOwn(t *testing.T) string {
	t.Helper()

	shared := t.TempDir()
	t.Setenv("ProgramData", shared)
	return filepath.Join(shared, "sshakku")
}

// F59: a machine-wide install without the authority to finish one creates
// nothing — least of all the directory whose owner is whoever makes it first,
// and whose contents every login on the machine runs.
func TestAMachineWideInstallWithoutTheAuthorityLeavesTheSharedDirectoryUnmade(t *testing.T) {
	hookDir := sharedDirectoryOfItsOwn(t)
	home := t.TempDir()
	installInto(t, home)
	asASessionThatMayChangeTheMachine(t, false)

	_, err := Install(t.Context(), machineRequest(t, home, filepath.Join(home, "profile.ps1")), Ancestry{})

	require.Error(t, err)
	assert.NoDirExists(t, hookDir, "a refused install does not leave the shared directory behind it")
}

// F59: and with that authority the hook goes into the directory the machine
// shares, which is what every account's login is pointed at.
func TestAMachineWideInstallPutsTheHookInTheDirectoryTheMachineShares(t *testing.T) {
	hookDir := sharedDirectoryOfItsOwn(t)
	home := t.TempDir()
	installInto(t, home)
	asASessionThatMayChangeTheMachine(t, true)

	out, err := Install(t.Context(), machineRequest(t, home, filepath.Join(home, "profile.ps1")), Ancestry{})

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(hookDir, "shell-hook.ps1"), out.HookFile,
		"the hook goes in the machine's own directory, not the account's")
	assert.FileExists(t, out.HookFile)
	// And nothing else does. What lives in a directory the whole machine can
	// read is a promise in its own right: an install that started keeping a
	// person's settings or their log beside the hook would put them where every
	// account can read them.
	shared, err := os.ReadDir(hookDir)
	require.NoError(t, err)
	assert.Equal(t, []string{"shell-hook.ps1"}, namesOf(shared),
		"only the hook, for an install that was told to record no search list")
}

// namesOf is what a directory holds, by name.
func namesOf(entries []os.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}
