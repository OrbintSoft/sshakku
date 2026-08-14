package handoff

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// saveRandSeam snapshots the token RNG seam, restoring it when the (sub)test
// ends.
func saveRandSeam(t *testing.T) {
	t.Helper()
	original := randRead
	t.Cleanup(func() { randRead = original })
}

func TestRandomHandoffTokenReadError(t *testing.T) {
	saveRandSeam(t)
	randRead = func([]byte) (int, error) { return 0, errors.New("rng boom") }
	_, err := randomToken()
	assert.Error(t, err,
		"a token that is not random is one another process can guess, so an RNG that failed must stop the handoff")
}

// TestFetchHandoffMalformedToken covers Fetch (and its platform
// Fetch) rejecting a token it cannot redeem: a non-numeric keyring
// serial on Linux, an undialable socket path on Darwin, and on Windows a
// handoff there is no mechanism for at all.
func TestFetchHandoffMalformedToken(t *testing.T) {
	_, err := Fetch(t.Context(), "definitely-not-a-valid-handoff-token")
	assert.Error(t, err, "a handle no stash was made under can redeem nothing")
}
