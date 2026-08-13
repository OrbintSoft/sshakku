package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/keys"
	"github.com/stretchr/testify/assert"
)

// recordingTTY is a terminal that answers, and remembers being asked. The answer
// is what someone would have typed at a prompt they never meant to reach, so a
// run that reaches the terminal shows up twice: in asked, and in whatever the
// run then does with the reply.
type recordingTTY struct {
	asked  []string
	answer string
}

func (t *recordingTTY) Prompt(prompt string, _ bool) (string, error) {
	t.asked = append(t.asked, prompt)
	return t.answer, nil
}

var _ keys.TTY = (*recordingTTY)(nil)

// wiredShell puts the test in the environment a login shell has once SSHakku is
// wired into it — the same variables askpass-env exports — so what runs
// afterwards is a command typed in that shell rather than one typed in a bare
// one.
func wiredShell(t *testing.T) {
	t.Helper()
	t.Setenv("SSH_ASKPASS", "/opt/sshakku/bin/"+askpassProgName)
	t.Setenv("SSH_ASKPASS_REQUIRE", "force")
}

// TestUnknownCommandIsNotASecretRequest verifies F30: a command SSHakku does not
// recognise is answered by naming it and showing what the commands are, never by
// asking for a secret — and being wired into the shell changes nothing about
// that.
//
// Both halves are asserted because a run can satisfy one and fail the other, and
// the second is the one that costs the user something: a terminal read with echo
// off swallows the next thing typed, and whatever it was is written to stdout as
// though it were the answer to a question ssh had asked.
func TestUnknownCommandIsNotASecretRequest(t *testing.T) {
	mistyped := [][]string{
		{"--forget"},
		{"frogget", "id_ed25519"},
	}
	for _, args := range mistyped {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			tempRuntimeEnv(t)
			wiredShell(t)

			tty := &recordingTTY{answer: "a passphrase nobody asked for"}
			d := depsReturning(newMemoryBackend())
			d.tty = tty

			var stdout, stderr bytes.Buffer
			code := dispatch(t.Context(), d, &stdout, &stderr, "/usr/local/bin/sshakku", args)

			assert.Emptyf(t, tty.asked,
				"a command SSHakku does not know is not a question to put to the user: %q", tty.asked)
			assert.Empty(t, stdout.String(),
				"and whatever is written there is read by ssh as a secret")
			assert.Equal(t, 2, code, "an unrecognised command is a usage error")
			assert.Contains(t, stderr.String(), args[0], "the answer must name what was actually typed")
			assert.Contains(t, stderr.String(), "usage: sshakku", "and show what the commands are")
		})
	}
}
