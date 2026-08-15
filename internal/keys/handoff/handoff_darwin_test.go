//go:build darwin

package handoff

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStashAndFetchRoundTripOnThisSystem covers what Stash itself decides,
// which is where the socket goes on this system. Everything it delegates to is
// checked elsewhere against a base a test names; what is checked here is that
// the base this platform actually picks yields an address the kernel accepts.
//
// It is a round trip because a stash whose address was too long, or landed
// somewhere unwritable, fails at bind — and a passphrase that cannot be handed
// over is a login that stops to ask for one it already had.
func TestStashAndFetchRoundTripOnThisSystem(t *testing.T) {
	const passphrase = "s3cr3t"

	token, err := Stash(passphrase, 5*time.Second)
	require.NoError(t, err, "putting a passphrase aside must succeed under the directory this system chooses")
	require.NotEmpty(t, token, "and must name where to collect it from")

	got, err := Fetch(t.Context(), token)
	require.NoError(t, err, "and collecting it must succeed")
	assert.Equal(t, passphrase, got, "the passphrase handed over must be the one put aside")

	_, err = Fetch(t.Context(), token)
	assert.Error(t, err, "and it must be collectable once: a passphrase left waiting is one anyone could still collect")
}
