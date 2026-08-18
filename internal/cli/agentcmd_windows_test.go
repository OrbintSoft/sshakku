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
// system's own agent, in the writing the shell being printed for reads — or, on
// a machine whose agent service cannot be started at all, says so and names the
// command that would put it right.
//
// Which of the two a machine gives is the machine's, not this test's: a runner
// with the service disabled is a real Windows and the second promise is the one
// that applies there. So both are asserted, each in full, and anything else —
// a silent failure, an empty print, a message naming nothing to do — fails.
//
// Nothing is faked. The ensurer is the one the product composes for this
// platform, which is the decision under test, and the endpoint printed is the
// one a session will really be handed.
func TestOnThisSystemAWiredShellIsPointedAtTheSystemsOwnAgentOrToldWhyNot(t *testing.T) {
	tempRuntimeEnv(t)
	var stdout, stderr bytes.Buffer

	code := realDeps().run(t.Context(), &stdout, &stderr, []string{"shell-init"})

	assertEndpointOrRemedy(t, code, stdout.String(), stderr.String(),
		"agent_sock='"+agent.SystemEndpoint().ForPosixShell()+"'")
	if code == 0 {
		assert.Contains(t, stdout.String(), "log_file=", "the other paths come with it")
		logged, err := os.ReadFile(paths.Resolve(paths.FromOS(), paths.ProbeDir).LogFile)
		require.NoError(t, err, "the session log is written either way")
		assert.NotContains(t, string(logged), "keeping an ssh-agent on a fixed endpoint",
			"an agent can be kept here now, and a log still saying otherwise would send somebody looking for a fault")
	}
}

// F50: the same endpoint, handed to a shell of this system's own, arrives in
// this system's own writing — which is the one that survives being typed, read
// back, and compared against what any other native program shows. Where no
// agent can be had at all, the promise is again the message rather than a
// session pointed at silence.
func TestAShellOfThisSystemIsHandedTheSystemsOwnWriting(t *testing.T) {
	tempRuntimeEnv(t)
	var stdout, stderr bytes.Buffer

	code := realDeps().run(t.Context(), &stdout, &stderr, []string{"shell-init", "--shell=powershell"})

	assertEndpointOrRemedy(t, code, stdout.String(), stderr.String(),
		"$agent_sock = '"+agent.SystemEndpoint().Native()+"'")
}

// assertEndpointOrRemedy holds the command to whichever of the two promises
// this machine can keep: the endpoint printed for the shell that asked, or a
// refusal naming the command that would end it. Neither is a weaker version of
// the other, and a run that does something else is a failure.
func assertEndpointOrRemedy(t *testing.T, code int, stdout, stderr, wantAssignment string) {
	t.Helper()
	if code == 0 {
		assert.Empty(t, stderr, "a session that got its agent opens with nothing printed")
		assert.Contains(t, stdout, wantAssignment, "and is pointed at the endpoint, in the writing it reads")
		return
	}
	require.Equalf(t, 1, code, "an agent that could not be had is a failed init, not a crash: %s", stderr)
	assert.Regexp(t, `Set-Service|Start-Service|Add-WindowsCapability`, stderr,
		"a refusal a person cannot act on is half a message: the command that ends it is named")
	assert.NotContains(t, stdout, "agent_sock",
		"and no session is pointed at an endpoint nothing was shown to answer on")
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
