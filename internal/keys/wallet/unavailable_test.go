package wallet

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnavailableBackendFailsEveryOperationWithItsReason(t *testing.T) {
	reason := errors.New("the secret-service route needs an API this platform has none of")
	b := Unavailable{Reason: reason}

	_, _, err := b.Lookup(t.Context(), "k")
	assert.ErrorIs(t, err, reason,
		"a miss would send the loader off to prompt with no explanation of why the wallet was never consulted")
	assert.ErrorIs(t, b.Store(t.Context(), "k", "k", "p"), reason, "and a store must not be reported as having landed anywhere")
	assert.ErrorIs(t, b.Delete(t.Context(), "k"), reason, "nor a removal as having happened")
	_, err = b.List(t.Context())
	assert.ErrorIs(t, err, reason, "nor the wallet as holding nothing")
}

// TestUnavailableBackendLookupIsNotAMiss states the distinction the type
// exists for: reporting "nothing stored" would let a later store overwrite
// whatever is really in the wallet.
func TestUnavailableBackendLookupIsNotAMiss(t *testing.T) {
	_, found, err := Unavailable{Reason: errors.New("unreachable")}.Lookup(t.Context(), "k")
	assert.False(t, found,
		"a wallet nobody reached must not claim to have looked: a later store would overwrite what is really in it")
	assert.Error(t, err, "and must say why it was not reached")
}
