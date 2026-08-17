//go:build windows

package cli

import (
	"bytes"
	"os"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/paths"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// F48, F10: on this system there is no ssh-agent for SSHakku to keep, and the
// command every wired login runs must still open the shell.
//
// Nothing is faked here. The ensurer is the one the product composes for this
// platform, which is the decision under test: a fake would answer the question
// this test exists to ask. There is no counterpart on the other platform,
// because there the same call really starts an agent — which is that platform's
// own job and not this one's.
func TestOnThisSystemAWiredShellStillOpensWithNothingSaid(t *testing.T) {
	tempRuntimeEnv(t)
	var stdout, stderr bytes.Buffer

	code := realDeps().run(t.Context(), &stdout, &stderr, []string{"shell-init"})

	require.Equalf(t, 0, code, "a shell must open on a system with no agent to give it: %s", stderr.String())
	assert.Empty(t, stderr.String(), "and open with nothing printed")
	assert.Contains(t, stdout.String(), "agent_sock=''",
		"the hook reads an empty socket as there being none, and leaves the session's own alone")
	assert.Contains(t, stdout.String(), "log_file=", "the paths this system does have are still handed over")

	// And what was written down is the mechanism that is absent, not whichever
	// step of a lifecycle that cannot run here happened to be reached first.
	// Both answers open the shell, so only the log tells them apart — and the
	// one a person reads is the one that has to be true.
	logged, err := os.ReadFile(paths.Resolve(paths.FromOS(), paths.ProbeDir).LogFile)
	require.NoError(t, err, "the absence must still be written down")
	assert.Contains(t, string(logged), "keeping an ssh-agent on a fixed endpoint")
	assert.NotContains(t, string(logged), "locking the agent start path",
		"a lock is a step of keeping an agent, and naming it would send somebody looking at the wrong thing")
}
