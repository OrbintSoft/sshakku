package cli

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// F52: where the agent holds no lifetimes, a key is added without one rather
// than not added at all — asking for a lifetime there is refused outright, so
// the choice is between a key that loads and no key. Both answers are checked
// from either machine, since which one a system gives is the system's own.
func TestAKeyLoadsEvenWhereTheAgentHoldsNoLifetimes(t *testing.T) {
	configured := 8 * time.Hour

	assert.Equal(t, configured, effectiveKeyLifetime(configured, true),
		"where the agent holds lifetimes, the configured one is what the key is added with")
	assert.Zero(t, effectiveKeyLifetime(configured, false),
		"where it holds none, the key is added with no expiry rather than refused")
	assert.Zero(t, effectiveKeyLifetime(0, true),
		"no expiry asked for is no expiry, wherever this runs")
}
