//go:build unix

package install

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Where an install writes on this system, asked of the table itself rather than
// through an install: these are the directories `make install` and
// `make install-user` already use, and a wiring done by this program has to land
// where a wiring done by those would.
func TestThisSystemsInstallLocations(t *testing.T) {
	env := environment(map[string]string{"HOME": "/home/example"})

	user, err := locationsFor(User, env)
	require.NoError(t, err)
	assert.Equal(t, "/home/example/.local/bin", user.BinDir)
	assert.Equal(t, "/home/example/.local/share/sshakku", user.HookDir)

	machine, err := locationsFor(Machine, env)
	require.NoError(t, err)
	assert.Equal(t, "/usr/local/bin", machine.BinDir)
	assert.Equal(t, "/usr/local/share/sshakku", machine.HookDir)
}

// A machine-wide install is a property of the system's layout, so an
// environment cannot redirect one — which is the whole reason it is not read
// from a variable.
func TestAMachineInstallGoesTheSamePlaceWhateverTheEnvironmentSays(t *testing.T) {
	redirected, err := locationsFor(Machine, environment(map[string]string{
		"HOME": "/home/example", "PREFIX": "/tmp/mine", "DESTDIR": "/tmp/mine",
	}))

	require.NoError(t, err)
	assert.Equal(t, "/usr/local/bin", redirected.BinDir)
}

// An account with no home directory in its environment has no install location,
// and is told so by the name of what is missing. An empty string silently joined
// onto would make `.local/bin` — a relative path, resolved against whatever
// directory the install was run from, created without complaint and read by no
// session.
func TestWithNoHomeInTheEnvironmentThereIsNowhereToInstall(t *testing.T) {
	for _, env := range []map[string]string{{}, {"HOME": ""}} {
		_, err := locationsFor(User, environment(env))

		require.Error(t, err)
		assert.Contains(t, err.Error(), "HOME", "the name of what is missing is the whole of the remedy")
	}
}

func TestAScopeThisSystemDoesNotServeHasNoLocations(t *testing.T) {
	_, err := locationsFor(Scope("everyone"), environment(map[string]string{"HOME": "/home/example"}))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "everyone")
}
