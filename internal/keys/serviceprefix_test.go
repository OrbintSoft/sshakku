package keys

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/OrbintSoft/sshakku/internal/keys/wallet"
)

// TestServicePrefixOfConfig covers the step between a configuration and the
// name an entry is written under: a prefix the user set is the one used, and an
// unset one resolves the same way it does everywhere else, because it is
// resolved in only one place.
func TestServicePrefixOfConfig(t *testing.T) {
	assert.Equal(t, "chosen", servicePrefixOf(Config{ServicePrefix: "chosen"}),
		"a prefix the user configured is what their entries are named with")
	assert.Equal(t, wallet.DefaultServicePrefix, servicePrefixOf(Config{}),
		"and one they never set resolves where every other unset prefix does, "+
			"so a store that writes an entry and a sweep that enumerates one cannot disagree")
}
