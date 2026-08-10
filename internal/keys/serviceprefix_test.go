package keys

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOwnServicesGoesByThePrefixItIsGiven pins the half of the entry name that
// says who stored it. The case that matters is the second one: under a
// configuration naming one prefix, an entry carrying a different one is not
// sshakku's — not even the default's — because nothing this configuration can
// write would ever have produced it.
func TestOwnServicesGoesByThePrefixItIsGiven(t *testing.T) {
	const mine = "wallet-of-mine"
	names := []string{
		"github.com",
		mine + "-id_ed25519",
		defaultServicePrefix + "-id_rsa",
		"Passport scan",
	}

	t.Run("a configured prefix keeps its own entries and no others", func(t *testing.T) {
		assert.Equal(t, []string{mine + "-id_ed25519"}, ownServices(names, mine),
			"an entry under a different prefix is not this configuration's, not even the default's: "+
				"nothing this configuration can write would ever have produced it")
	})

	t.Run("no prefix configured falls back to the default", func(t *testing.T) {
		assert.Equal(t, []string{defaultServicePrefix + "-id_rsa"}, ownServices(names, ""),
			"a user who configured nothing keeps the entries SSHakku already wrote for them")
	})
}

// TestServicePrefixOrDefault covers the single place that decides what an unset
// prefix means, which is why writing an entry and enumerating one cannot
// disagree about it.
func TestServicePrefixOrDefault(t *testing.T) {
	assert.Equal(t, defaultServicePrefix, servicePrefixOrDefault(""),
		"one place decides what an unset prefix means, so writing an entry and enumerating one cannot disagree")
	assert.Equal(t, "chosen", servicePrefixOrDefault("chosen"), "and a configured one is kept as written")
	assert.Equal(t, "chosen", servicePrefixOf(Config{ServicePrefix: "chosen"}),
		"including when it is read off the configuration")
}

// TestBitwardenListGoesByTheBackendsOwnPrefix verifies F32 where the vault is
// shared with everything else its owner keeps there: what List reports is what
// `forget --all` deletes, so it must report the entries the configured prefix
// names and nothing else — including nothing under the default, which under
// this configuration is another program's name as much as "Bank" is.
func TestBitwardenListGoesByTheBackendsOwnPrefix(t *testing.T) {
	const mine = "wallet-of-mine"
	r := newFakeRunner().on(bitwardenBin, stdout(
		`[{"name":"Bank"},{"name":"`+mine+`-id_ed25519"},{"name":"`+defaultServicePrefix+`-id_rsa"}]`, 0))
	b := &BitwardenBackend{Runner: r, Session: "sess-token", held: true, ServicePrefix: mine}

	got, err := b.List()
	require.NoError(t, err, "listing the vault must succeed")
	assert.Equal(t, []string{mine + "-id_ed25519"}, got,
		"under this configuration the default prefix is another program's name as much as \"Bank\" is, "+
			"and what List reports is what forget --all deletes")
}
