//go:build windows

package cli

import (
	"bytes"
	"os"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/agent"
	"github.com/OrbintSoft/sshakku/internal/paths"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// F50, F51, F10: the command every wired login runs points the session at this
// system's own agent, in the writing the shell being printed for reads.
//
// Nothing is faked. The ensurer is the one the product composes for this
// platform, which is the decision under test — a fake would answer the question
// this test exists to ask — and the endpoint printed is the one a session will
// really be handed. If the service is not running when this runs, starting it
// is the promise being exercised rather than a side effect being tolerated.
func TestOnThisSystemAWiredShellIsPointedAtTheSystemsOwnAgent(t *testing.T) {
	tempRuntimeEnv(t)
	var stdout, stderr bytes.Buffer

	code := realDeps().run(t.Context(), &stdout, &stderr, []string{"shell-init"})

	require.Equalf(t, 0, code, "a wired login must open with a working agent: %s", stderr.String())
	assert.Empty(t, stderr.String(), "and open with nothing printed")
	assert.Contains(t, stdout.String(), "agent_sock='"+agent.SystemEndpoint().ForPosixShell()+"'",
		"a shell that said nothing about its language is a POSIX one, and reads the writing it can carry")
	assert.Contains(t, stdout.String(), "log_file=", "the other paths come with it")

	logged, err := os.ReadFile(paths.Resolve(paths.FromOS(), paths.ProbeDir).LogFile)
	require.NoError(t, err, "the session log is written either way")
	assert.NotContains(t, string(logged), "keeping an ssh-agent on a fixed endpoint",
		"an agent can be kept here now, and a log still saying otherwise would send somebody looking for a fault")
}

// F50: the same endpoint, handed to a shell of this system's own, arrives in
// this system's own writing — which is the one that survives being typed, read
// back, and compared against what any other native program shows.
func TestAShellOfThisSystemIsHandedTheSystemsOwnWriting(t *testing.T) {
	tempRuntimeEnv(t)
	var stdout, stderr bytes.Buffer

	code := realDeps().run(t.Context(), &stdout, &stderr, []string{"shell-init", "--shell=powershell"})

	require.Equalf(t, 0, code, "a wired login must open with a working agent: %s", stderr.String())
	assert.Contains(t, stdout.String(), "$agent_sock = '"+agent.SystemEndpoint().Native()+"'",
		"PowerShell reads the backslash writing, and reads it as an assignment")
}

// F48 is answered here by the composition rather than by a lifecycle failing at
// whichever step it reaches first: what drives the agent on this system is the
// service lifecycle, and it is what the product really builds.
func TestThisSystemsLifecycleIsTheServiceOne(t *testing.T) {
	assert.IsType(t, agent.ServiceAgent{}, platformEnsurer(),
		"an agent here is a service on an endpoint of the system's own")
	assert.IsType(t, agent.ServiceAgent{}, realEnsurer(),
		"and that is what the product composes here")
}
