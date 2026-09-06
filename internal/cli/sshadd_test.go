package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/agent"
	"github.com/OrbintSoft/sshakku/internal/cli/shell"
	"github.com/OrbintSoft/sshakku/internal/paths"
)

// errNoBuildReachesTheAgent is what a session whose only OpenSSH cannot open
// this system's agent is answered with.
var errNoBuildReachesTheAgent = errors.New("no build of ssh-add here can reach this system's agent")

// TestEveryCommandHereIsToldWhichSSHAddToRun. The choosing is done once, in the
// wiring, and a command left to fall back to the ordinary name would — on a
// system where a shell can bring an OpenSSH of its own — hand a passphrase to a
// build that reaches no agent, and be told the passphrase was wrong.
func TestEveryCommandHereIsToldWhichSSHAddToRun(t *testing.T) {
	require.NotNil(t, realDeps().sshAdd,
		"nothing else in this package asks which ssh-add can reach the agent")
}

// TestAShellIsHandedTheToolsThatGoWithTheEndpoint. The endpoint and the tools
// are one thing: a session pointed at an agent its own ssh cannot open has been
// pointed at nothing, and every `ssh` typed there — and every one `git` starts
// on the user's behalf — asks for a passphrase however full the agent is.
func TestAShellIsHandedTheToolsThatGoWithTheEndpoint(t *testing.T) {
	tempRuntimeEnv(t)
	theOnesThatCanReachIt := filepath.Join(string(filepath.Separator), "c", "Windows", "System32", "OpenSSH")

	d := depsWithEnsurer(fakeEnsurer{res: agent.EnsureResult{Live: agent.SocketEndpoint("/run/sshakku/agent.sock")}})
	d.sshToolsDir = func(context.Context, shell.Dialect) (string, error) {
		return theOnesThatCanReachIt, nil
	}

	var out, errOut bytes.Buffer
	require.Zerof(t, d.shellInit(t.Context(), &out, &errOut, nil), "shellInit must succeed; stderr=%q", errOut.String())

	assert.Contains(t, out.String(), "ssh_tools_dir='"+theOnesThatCanReachIt+"'",
		"the shell has to be told where they are, in the writing it reads")
}

// TestAShellThatAlreadyRunsToolsReachingTheAgentIsToldToChangeNothing. The
// assignment is still printed, so a hook reads an answer rather than an absence
// — but it names no directory, because putting one ahead of somebody's own PATH
// for the rest of their login is not a thing to do for no outcome.
func TestAShellThatAlreadyRunsToolsReachingTheAgentIsToldToChangeNothing(t *testing.T) {
	tempRuntimeEnv(t)
	d := depsWithEnsurer(fakeEnsurer{res: agent.EnsureResult{Live: agent.SocketEndpoint("/run/sshakku/agent.sock")}})

	var out, errOut bytes.Buffer
	require.Zerof(t, d.shellInit(t.Context(), &out, &errOut, nil), "shellInit must succeed; stderr=%q", errOut.String())

	assert.Contains(t, out.String(), "ssh_tools_dir=''")
}

// TestAShellThatCannotBeGivenReachingToolsOpensAnyway, and the reason goes in
// the session log. This runs before a person's first prompt: a shell that
// refused to open over it would be worse than the thing it is reporting, and a
// line on their screen at every login is not how they would want to hear it.
func TestAShellThatCannotBeGivenReachingToolsOpensAnyway(t *testing.T) {
	tempRuntimeEnv(t)
	layout := paths.Resolve(paths.FromOS(), paths.ProbeDir).WithSocketToken(paths.SocketToken())

	d := depsWithEnsurer(fakeEnsurer{res: agent.EnsureResult{Live: agent.SocketEndpoint("/run/sshakku/agent.sock")}})
	d.sshToolsDir = func(context.Context, shell.Dialect) (string, error) {
		return "", errNoBuildReachesTheAgent
	}

	var out, errOut bytes.Buffer
	require.Zero(t, d.shellInit(t.Context(), &out, &errOut, nil), "the session opens regardless")
	assert.Empty(t, errOut.String(), "not on the screen of every login")
	assert.Contains(t, out.String(), "ssh_tools_dir=''", "and nothing is put in front of the session's own PATH")

	logged, err := os.ReadFile(layout.LogFile)
	require.NoError(t, err, "the session log must have been written")
	assert.Contains(t, string(logged), errNoBuildReachesTheAgent.Error())
}

// TestASessionWithNoUsableSSHAddIsToldRatherThanLeftToRetry drives load-keys
// on an account that has a key, in a session whose only OpenSSH cannot reach
// the agent. What must not happen is the load going ahead: every passphrase
// would come back as wrong, the user would be asked for a correct one until
// the attempts ran out, and the key would then be given up on for every later
// shell — none of which says anything about the program that was run.
func TestASessionWithNoUsableSSHAddIsToldRatherThanLeftToRetry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_STATE_HOME", home)
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".ssh"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".ssh", "id_ed25519"), []byte("a key"), 0o600))

	logFile := paths.Resolve(paths.FromOS(), paths.ProbeDir).LogFile
	require.NoError(t, os.MkdirAll(filepath.Dir(logFile), 0o700), "seed the directory the session log lives in")

	d := depsReturning(newMemoryBackend())
	d.sshAdd = func() (string, error) { return "", errNoBuildReachesTheAgent }

	var errOut bytes.Buffer
	assert.Equal(t, 1, d.loadKeys(t.Context(), &errOut),
		"a session that cannot reach its agent did not load the keys, and must not report that it did")
	assert.Contains(t, errOut.String(), errNoBuildReachesTheAgent.Error(),
		"the user is at a terminal and this is the moment to tell them")

	logged, err := os.ReadFile(logFile)
	require.NoError(t, err, "the session log must have been written")
	assert.Contains(t, string(logged), errNoBuildReachesTheAgent.Error(),
		"and it belongs in the log too, for the sessions where nobody is reading a terminal")
}
