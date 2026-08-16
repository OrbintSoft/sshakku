package install

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// environment stands in for the process's own, so what a table does with a
// variable that is missing can be asked without unsetting anything real.
func environment(pairs map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		value, ok := pairs[name]
		return value, ok
	}
}

// Whatever this platform's table says, these hold of it. They are the promises
// the rest of the install is written against, and they are asked of the table
// that is really compiled in rather than of a copy of it.
func TestWhereverThisSystemInstalls(t *testing.T) {
	for _, scope := range []Scope{User, Machine} {
		t.Run(string(scope), func(t *testing.T) {
			where, err := LocationsFor(scope)
			require.NoError(t, err, "this system has to be able to say where its own install goes")

			assert.NotEmpty(t, where.BinDir)
			assert.NotEmpty(t, where.HookDir)
			assert.True(t, filepath.IsAbs(where.BinDir),
				"a relative directory resolves against wherever the command was run from; got %q", where.BinDir)
			assert.True(t, filepath.IsAbs(where.HookDir), "and so would this one; got %q", where.HookDir)
			assert.NotEqual(t, where.BinDir, where.HookDir,
				"a directory of executables and a directory of rendered data are not the same directory")
			assert.Contains(t, strings.ToLower(where.HookDir), "sshakku",
				"the hook goes somewhere this program owns, not loose in a shared directory")
		})
	}
}

// The two scopes must never resolve to the same place: a user install that
// reached a machine directory would fail for an account that cannot write
// there, and succeed — by wiring every account — for one that can.
func TestAUserInstallAndAMachineInstallGoToDifferentPlaces(t *testing.T) {
	mine, err := LocationsFor(User)
	require.NoError(t, err)
	everyones, err := LocationsFor(Machine)
	require.NoError(t, err)

	assert.NotEqual(t, mine.BinDir, everyones.BinDir)
	assert.NotEqual(t, mine.HookDir, everyones.HookDir)
}

// A variable that is not set is an error naming it, never an empty string
// joined onto. Joined, `%LOCALAPPDATA%\sshakku` becomes a relative `sshakku`
// that resolves against whatever directory the install ran from: a plausible
// path, created without complaint, that no session will ever read.
func TestADirectoryWithNothingToBuildItFromIsRefused(t *testing.T) {
	for name, lookup := range map[string]func(string) (string, bool){
		"the variable is not there": environment(nil),
		"the variable is empty":     environment(map[string]string{"HOME": "", "LOCALAPPDATA": ""}),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := directory(lookup, "LOCALAPPDATA", "sshakku")

			require.Error(t, err)
			assert.Contains(t, err.Error(), "LOCALAPPDATA", "the message names the variable that was missing")
			assert.Contains(t, err.Error(), "not set")
		})
	}
}

func TestADirectoryIsBuiltUnderTheVariableItWasGiven(t *testing.T) {
	got, err := directory(environment(map[string]string{"BASE": filepath.FromSlash("/base")}), "BASE", "one", "two")

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(filepath.FromSlash("/base"), "one", "two"), got)
}

// A scope nobody serves is refused with what there is, rather than answered
// with one of the two: installing to the wrong scope is worse than not
// installing.
func TestAScopeThatIsNotOfferedIsRefusedAndNamesWhatThereIs(t *testing.T) {
	for _, scope := range []Scope{"", "everyone", "USER", "root"} {
		_, err := LocationsFor(scope)

		require.Error(t, err, "scope %q", scope)
		assert.Contains(t, err.Error(), string(User))
		assert.Contains(t, err.Error(), string(Machine))
	}
}

// The startup files a Bourne shell reads are named in the shell's spelling,
// which on the platform this matters for is not this program's. Joined with a
// backslash the file is still created — under a name the shell never opens.
func TestABourneStartupFileIsNamedTheWayTheShellNamesIt(t *testing.T) {
	login, err := BourneLoginFile("/c/Users/example")
	require.NoError(t, err)
	rc, err := BourneRCFile("/c/Users/example")
	require.NoError(t, err)

	assert.Equal(t, "/c/Users/example/.bash_profile", login)
	assert.Equal(t, "/c/Users/example/.bashrc", rc)
	assert.NotContains(t, login, `\`)
	assert.NotContains(t, rc, `\`)
}

func TestAShellWithNoHomeHasNoStartupFileToName(t *testing.T) {
	_, err := BourneLoginFile("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no startup file")

	_, err = BourneRCFile("")
	require.Error(t, err)
}

// The machine-wide directory is named in the shell's spelling too, and
// deliberately not as this platform's own /etc: under a POSIX-emulating
// environment it is that environment's, wherever it was installed, and the
// translator is what turns it into a real directory.
func TestTheMachineWideDirectoryIsTheShellsAndNotTheSystems(t *testing.T) {
	assert.Equal(t, "/etc/profile.d", MachineDropInDir)
	assert.NotContains(t, MachineDropInDir, `\`)
}
