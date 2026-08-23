package cli

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// F52: where the agent holds no lifetimes, a key is added without one rather
// than not added at all — asking for a lifetime there is refused outright, so
// the choice is between a key that loads and no key. F53: the lifetime is still
// the key's, because what the record keeps is what a later session goes by when
// it takes an expired key back out. Both answers are checked from either
// machine, since which one a system gives is the system's own.
func TestAKeyLoadsEvenWhereTheAgentHoldsNoLifetimes(t *testing.T) {
	configured := 8 * time.Hour

	told, recorded := keyLifetimes(configured, true)
	assert.Equal(t, configured, told,
		"where the agent holds lifetimes, the configured one is what the key is added with")
	assert.Equal(t, configured, recorded,
		"and the record says the same, since the agent is what will enforce it")

	told, recorded = keyLifetimes(configured, false)
	assert.Zero(t, told,
		"where it holds none, the key is added with no expiry rather than refused")
	assert.Equal(t, configured, recorded,
		"but the record still keeps the configured lifetime: with a zero there, nothing would ever be expired")

	told, recorded = keyLifetimes(0, true)
	assert.Zero(t, told, "no expiry asked for is no expiry, wherever this runs")
	assert.Zero(t, recorded, "and nothing to record either")
}
