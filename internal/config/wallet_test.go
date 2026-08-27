package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The tests here ask questions every platform can answer, so none of them
// carries a build tag. They were held to the unix family while Windows had no
// wallet to name — a list with nothing in it, and a report printing an empty
// value where the wallet in force belongs. That is no longer true of any
// platform this builds for, and a test of what every system does is only that
// if every system runs it.

// TestSecretBackendsIsTheOneList guards what the exported answers are for:
// every caller that offers the user a choice of wallet, or turns one down,
// reads them here instead of keeping a copy of its own. Two copies is how the
// diagnostics came to accept a name the configuration would not.
func TestSecretBackendsIsTheOneList(t *testing.T) {
	names := SecretBackends()
	require.NotEmpty(t, names, "this system offers no wallet at all")
	for _, name := range names {
		assert.Truef(t, SecretBackendAvailable(name),
			"SecretBackendAvailable(%q) = false for a wallet the same package offers", name)
		s, errs := Resolve(File{SecretBackend: new(name)}, lookupFrom(nil))
		assert.Emptyf(t, errs, "naming the offered wallet %q must not be reported", name)
		assert.Equalf(t, name, s.SecretBackend, "an offered wallet must be accepted")
	}
	assert.False(t, SecretBackendAvailable("bogus"), `SecretBackendAvailable("bogus")`)
	assert.Truef(t, SecretBackendAvailable(DefaultSecretBackend()),
		"the default wallet %q is not among the ones offered", DefaultSecretBackend())

	// The returned slice is the caller's own: handing out the live one would
	// let any caller reorder or empty the list every other caller reads.
	names[0] = "tampered"
	assert.NotEqual(t, "tampered", SecretBackends()[0],
		"SecretBackends hands out the list itself, so a caller can change what every other caller sees")
}

// TestEverySettingRendersAValueOnAMachineWithNoConfiguration covers the part of
// F35's report that is easiest to break: the report is read as a statement of
// what is in force, so a line showing nothing where a built-in value is at work
// reads as "off" or "none". The account that has written no configuration at
// all is the one most likely to be reading the report to find out what SSHakku
// does, and several settings answer that with something other than their own
// zero — a lifetime of zero means no expiry, an empty list of patterns means
// the built-in ones.
func TestEverySettingRendersAValueOnAMachineWithNoConfiguration(t *testing.T) {
	settings, errs := Resolve(File{}, func(string) (string, bool) { return "", false })
	require.Empty(t, errs, "resolving an empty configuration must report nothing")

	for _, desc := range settingTable {
		assert.NotEmptyf(t, desc.value(settings),
			"%s renders nothing where nobody has configured anything, want the value in force spelled out", desc.key)
	}
}
