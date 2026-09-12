//go:build linux

package handoff

import (
	"testing"
	"time"

	"github.com/OrbintSoft/sshakku/internal/keyring"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// F7: the passphrase never travels through an environment variable, a command
// line, or a file on disk. What crosses is a handle, and this is the pair that
// makes the handle worth anything, run against the real kernel keyring with
// nothing stood in for.
//
// The two halves live one process apart: a login stashes the passphrase and
// hands the token to the askpass helper ssh-add starts, and the helper redeems
// it. Every seam between them is injectable, so each half can be checked on its
// own — and neither check can say that the token one half writes is a token the
// other half accepts, which is the whole of what has to be true for a key to
// load without anybody being asked.
func TestAPassphrasePutAsideIsCollectedByTheTokenItWasGiven(t *testing.T) {
	if !keyring.Available() {
		t.Skip("kernel user keyring isn't usable for a round trip in this environment" +
			" (e.g. no session-keyring link — common in CI/containers without a PAM login)")
	}

	const passphrase = "sshakku-handoff-round-trip-passphrase" //nolint:gosec // G101 is right that this is a passphrase: the one this test invents for a key it invents

	token, err := Stash(passphrase, time.Minute)
	require.NoError(t, err, "putting a passphrase aside has to work, or ssh-add prompts on a terminal nobody is watching")

	got, err := Fetch(t.Context(), token)

	require.NoError(t, err, "the token one half writes has to be one the other half accepts")
	assert.Equal(t, passphrase, got, "and what comes back is what was put aside")

	_, err = Fetch(t.Context(), token)
	assert.Error(t, err, "a handoff is one-shot: a passphrase left in the keyring is one nobody is watching")
}
