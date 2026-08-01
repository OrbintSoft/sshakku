package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/keys"
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
// one. It returns the environment marker dispatch is given, read the way main
// reads it.
func wiredShell(t *testing.T) bool {
	t.Helper()
	t.Setenv("SSH_ASKPASS", "/opt/sshakku/bin/sshakku")
	t.Setenv("SSH_ASKPASS_REQUIRE", "force")
	t.Setenv(keys.EnvAskpassMode, "1")
	return os.Getenv(keys.EnvAskpassMode) != ""
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
			askpassEnvSet := wiredShell(t)

			tty := &recordingTTY{answer: "a passphrase nobody asked for"}
			d := depsReturning(newMemoryBackend())
			d.tty = tty

			var stdout, stderr bytes.Buffer
			code := dispatch(d, &stdout, &stderr, args, askpassEnvSet)

			if len(tty.asked) != 0 {
				t.Errorf("the terminal was read %d time(s), for %q; a command SSHakku does not know is not a question to put to the user", len(tty.asked), tty.asked)
			}
			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want nothing; whatever is written there is read by ssh as a secret", stdout.String())
			}
			if code != 2 {
				t.Errorf("exit = %d, want 2 (usage error)", code)
			}
			if !strings.Contains(stderr.String(), args[0]) {
				t.Errorf("stderr = %q, want the command that was not recognised named in it", stderr.String())
			}
			if !strings.Contains(stderr.String(), "usage: sshakku") {
				t.Errorf("stderr = %q, want the usage, so the user can see what the commands are", stderr.String())
			}
		})
	}
}
