//go:build windows

package install

import (
	"maps"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Where an install writes on this system, asked of the table itself. None of
// these is a fixed path — a profile may be on another drive and a system may be
// installed somewhere other than C: — so what is checked is the shape each one
// is built into, from an environment written here.
func TestThisSystemsInstallLocations(t *testing.T) {
	env := environment(map[string]string{
		"LOCALAPPDATA": `C:\Users\example\AppData\Local`,
		"ProgramFiles": `C:\Program Files`,
		"ProgramData":  `C:\ProgramData`,
	})

	user, err := locationsFor(User, env)
	require.NoError(t, err)
	assert.Equal(t, `C:\Users\example\AppData\Local\Programs\sshakku`, user.BinDir,
		"a per-account program goes where this system keeps per-account programs")
	assert.Equal(t, `C:\Users\example\AppData\Local\sshakku`, user.HookDir,
		"and the hook beside it rather than inside it, so a directory of executables stays one")

	machine, err := locationsFor(Machine, env)
	require.NoError(t, err)
	assert.Equal(t, `C:\Program Files\sshakku`, machine.BinDir)
	assert.Equal(t, `C:\ProgramData\sshakku`, machine.HookDir)
	assert.NotEqual(t, filepath.Dir(user.HookDir), filepath.Dir(machine.HookDir),
		"what one account reads and what the whole machine reads are different places")
}

// F44: an install with nowhere to write says the name of what is missing, and
// says it before anything is written. Each variable the table names is asked
// for separately, because a directory silently joined onto an empty string is a
// relative path — created without complaint under whatever directory the
// install was run from, and read by no session ever.
func TestEachDirectoryAnInstallNeedsIsNamedWhenTheEnvironmentHasNotGotIt(t *testing.T) {
	full := map[string]string{
		"LOCALAPPDATA": `C:\Users\example\AppData\Local`,
		"ProgramFiles": `C:\Program Files`,
		"ProgramData":  `C:\ProgramData`,
	}
	cases := []struct {
		scope    Scope
		variable string
	}{
		{User, "LOCALAPPDATA"},
		{Machine, "ProgramFiles"},
		{Machine, "ProgramData"},
	}
	for _, tc := range cases {
		t.Run(tc.variable, func(t *testing.T) {
			for _, withoutIt := range []map[string]string{{}, {tc.variable: ""}} {
				env := maps.Clone(full)
				delete(env, tc.variable)
				maps.Copy(env, withoutIt)

				_, err := locationsFor(tc.scope, environment(env))

				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.variable,
					"the name of what is missing is the whole of the remedy")
			}
		})
	}
}

func TestAScopeThisSystemDoesNotServeHasNoLocations(t *testing.T) {
	_, err := locationsFor(Scope("everyone"), environment(map[string]string{
		"LOCALAPPDATA": `C:\Users\example\AppData\Local`,
	}))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "everyone")
}
