package keys

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// capturedSSHAddEnv runs AddWithAskpass with the command it would have run
// intercepted, and hands back the environment ssh-add was to be started with.
// Only the running is replaced; what is asserted — which variables survive into
// the child — is decided before that and is not stood in for.
func capturedSSHAddEnv(t *testing.T) []string {
	t.Helper()

	oldStash, oldRun := stashPass, runCmd
	t.Cleanup(func() { stashPass, runCmd = oldStash, oldRun })

	var got []string
	stashPass = func(string, time.Duration) (string, error) { return "a-token", nil }
	runCmd = func(cmd *exec.Cmd) error {
		got = cmd.Env
		return nil
	}

	_, err := ExecKeyAdder{AskpassProg: "sshakku"}.AddWithAskpass(t.Context(), "id_ed25519", "hunter2")
	require.NoError(t, err)
	return got
}

// envValue returns the value of name in a child environment, and whether it was
// there at all.
func envValue(env []string, name string) (string, bool) {
	for _, entry := range env {
		if after, found := strings.CutPrefix(entry, name+"="); found {
			return after, true
		}
	}
	return "", false
}

// TestTheChildKeepsEveryVariableThisSystemSaysItNeeds. The child is ssh-add,
// and the child ssh-add starts is SSHakku's own askpass helper, which has to
// find the handoff the passphrase is waiting in. A variable dropped here does
// not fail loudly: the helper answers with nothing, ssh-add reads that as a
// wrong passphrase, and what the user is told is that their stored passphrase
// has gone stale.
func TestTheChildKeepsEveryVariableThisSystemSaysItNeeds(t *testing.T) {
	for _, name := range platformChildEnv {
		t.Setenv(name, "value-of-"+name)
	}

	env := capturedSSHAddEnv(t)

	for _, name := range platformChildEnv {
		value, present := envValue(env, name)
		assert.Truef(t, present, "%s is named as needed on this system and did not reach the child", name)
		assert.Equalf(t, "value-of-"+name, value, "%s reached the child with the wrong value", name)
	}
}

// TestTheChildIsToldWhereToAskAndWithWhat: the three the caller sets itself,
// which are what turn ssh-add away from a terminal and towards the helper.
func TestTheChildIsToldWhereToAskAndWithWhat(t *testing.T) {
	env := capturedSSHAddEnv(t)

	prog, present := envValue(env, "SSH_ASKPASS")
	assert.True(t, present, "ssh-add was given no helper to ask")
	assert.Equal(t, "sshakku", prog)

	require_, present := envValue(env, "SSH_ASKPASS_REQUIRE")
	assert.True(t, present, "without this ssh-add prefers a terminal, and a terminal is what there may not be")
	assert.Equal(t, "force", require_)

	_, present = envValue(env, "SSHAKKU_HANDOFF_TOKEN")
	assert.True(t, present, "the helper has no way to find the passphrase without the token")
}

// TestTheChildIsGivenNothingItWasNotMeantToHave: the environment is built up
// from nothing rather than inherited and trimmed, so a variable is in it
// because something needs it and for no other reason.
func TestTheChildIsGivenNothingItWasNotMeantToHave(t *testing.T) {
	t.Setenv("SSHAKKU_SOMETHING_NOBODY_ASKED_FOR", "1")

	env := capturedSSHAddEnv(t)

	_, present := envValue(env, "SSHAKKU_SOMETHING_NOBODY_ASKED_FOR")
	assert.False(t, present, "the child inherited a variable nothing named")
}
