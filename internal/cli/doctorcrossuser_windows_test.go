//go:build windows

package cli

import (
	"bytes"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/diagnose"
	"github.com/stretchr/testify/assert"
)

// TestAskingForAnotherAccountHereIsRefusedByName verifies F61 on the platform
// this build does not implement cross-user diagnosis for: `--user` is refused in
// a sentence about this build, naming what answers the same question instead,
// rather than in whatever the account database last had to say.
//
// The three values asked for are deliberately unalike — a name nobody has,
// something that parses as a number, and a well-formed SID — because F61
// promises the account is not looked up at all. One answer for all three is what
// that looks like from outside. Three different answers is the account database
// talking, and a caller cannot tell from them whether the account exists, let
// alone that the flag was never going to work here.
func TestAskingForAnotherAccountHereIsRefusedByName(t *testing.T) {
	asked := []string{"no-such-account-xyzzy", "1000", "S-1-5-18"}
	answers := make([]string, 0, len(asked))

	for _, who := range asked {
		// The token source errors if it is reached: the refusal is owed before
		// anything goes looking for somebody else's session.
		d := doctorDeps(diagnose.Report{}, fakeTokenSource{err: errMustNotRun}, 1000)
		var out, errOut bytes.Buffer

		assert.Equalf(t, 2, d.doctor(t.Context(), &out, &errOut, []string{"--user", who}),
			"--user %q asks for something this build does not do here, which is a refusal", who)
		assert.Emptyf(t, out.String(), "--user %q must not print a report of anybody", who)

		said := errOut.String()
		assert.Containsf(t, said, "--user", "the refusal for %q names the flag it is about", who)
		assert.Containsf(t, said, "not implemented", "and says that this is what it is, for %q", who)
		assert.Containsf(t, said, "sshakku doctor", "and names what answers the same question, for %q", who)
		for _, leaked := range []string{"No mapping", "parse uid", "strconv", "security ID"} {
			assert.NotContainsf(t, said, leaked,
				"--user %q is answered in this program's words, not the operating system's (%q)", who, leaked)
		}

		answers = append(answers, said)
	}

	for _, got := range answers[1:] {
		assert.Equal(t, answers[0], got,
			"a name nobody has, a number and a SID are one refusal here: what is not implemented is the "+
				"flag, and nothing was looked up to find that out")
	}
}
