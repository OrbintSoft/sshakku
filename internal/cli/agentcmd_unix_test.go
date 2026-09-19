//go:build unix

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// F1, F2: what drives the agent on this system is the lifecycle that starts one
// on a socket of its own choosing, reaps what has died and adopts what it did
// not start. The counterpart on the other platform names its own, so neither
// answer is left to whichever machine happens to run the suite.
func TestThisSystemsLifecycleIsTheSocketOne(t *testing.T) {
	assert.IsType(t, agent.Manager{}, platformEnsurer(),
		"an agent here is a process on a socket, and there is a lifecycle to keep it")
	assert.IsType(t, agent.Manager{}, realEnsurer(),
		"and that is what the product composes here")
}

// TestShellInitSaysWhereTheEndpointWentInstead covers F65 for the login path:
// where the runtime directory the environment names is not this account's
// alone, the session's endpoint goes somewhere else, and the session log names
// both the directory that was asked for and the one used instead. A socket that
// quietly moved is the part a person cannot work back from.
func TestShellInitSaysWhereTheEndpointWentInstead(t *testing.T) {
	home := tempRuntimeEnv(t)
	// A runtime directory anybody on the machine may write: the mode alone
	// settles it, so the test needs no second account to arrange.
	theirs := t.TempDir()
	require.NoError(t, os.Chmod(theirs, 0o777), "make the runtime directory one this account does not have to itself") //nolint:gosec // G302: the mode under test is the point
	t.Setenv("XDG_RUNTIME_DIR", theirs)

	d := depsWithEnsurer(fakeEnsurer{res: agent.EnsureResult{Live: agent.SocketEndpoint("/run/sshakku/agent.sock")}})
	var out, errOut bytes.Buffer
	require.Zerof(t, d.shellInit(t.Context(), &out, &errOut, nil), "shellInit must succeed; stderr=%q", errOut.String())

	assert.Empty(t, errOut.String(),
		"this is not a thing to put on the screen at every login")

	logged, err := os.ReadFile(filepath.Join(home, ".local", "state", "sshakku", "sessions.log"))
	require.NoError(t, err, "the session log must exist")
	assert.Contains(t, string(logged), theirs,
		"the directory the environment asked for is named, so it can be found and fixed")
	assert.NotContains(t, string(logged), "[ERROR]",
		"a runtime directory that was not used is not a failure of this session")
}
