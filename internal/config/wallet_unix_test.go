//go:build unix

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The tests here all rest on the same precondition: this system has at least
// one wallet SSHakku can be pointed at. Every unix does; Windows does not yet
// — platformSecretBackends there is empty, because the Credential Manager is
// not written and a backend reached through a tool of its own has not been
// shown to work there. They are held to this family until that changes, rather
// than rewritten to accommodate a platform with no wallet and rewritten back
// when it has one.
//
// One thing they were catching is genuinely broken on Windows and is not
// fixed by moving them: `sshakku config` prints an empty value for
// secret_backend there, and F35 promises the value in force spelled out —
// a blank line reads as "off". Closing that is the wallet step's job, since
// what it needs is a wallet to name.

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

func TestResolveSecretBackendAccountFieldsPassThrough(t *testing.T) {
	file := File{
		SecretBackend:    new(SecretBackendBitwarden),
		OnePasswordVault: new("sshakku-vault"),
		BitwardenEmail:   new("user@example.invalid"),
		BitwardenServer:  new("https://vault.example.invalid"),
	}
	s, errs := Resolve(file, lookupFrom(nil))
	require.Empty(t, errs, "unexpected errors")
	assert.Equal(t, "sshakku-vault", s.OnePasswordVault, "OnePasswordVault")
	assert.Equal(t, "user@example.invalid", s.BitwardenEmail, "BitwardenEmail")
	assert.Equal(t, "https://vault.example.invalid", s.BitwardenServer, "BitwardenServer")
}

func TestResolveAcceptsKeePassXCAsABackend(t *testing.T) {
	s, errs := Resolve(File{SecretBackend: new(SecretBackendKeePassXC)}, lookupFrom(nil))
	require.Empty(t, errs, "unexpected errors")
	assert.Equal(t, SecretBackendKeePassXC, s.SecretBackend, "the wallet is named, not the mechanism")
}

func TestResolveCarriesTheKeePassXCSettings(t *testing.T) {
	s, errs := Resolve(File{
		SecretBackend:     new(SecretBackendKeePassXC),
		KeePassXCRoute:    new(KeePassXCRouteCLI),
		KeePassXCDatabase: new("/home/someone/secrets.kdbx"),
		KeePassXCKeyFile:  new("/home/someone/secrets.key"),
	}, lookupFrom(nil))
	require.Empty(t, errs, "unexpected errors")
	assert.Equal(t, KeePassXCRouteCLI, s.KeePassXCRoute, "route")
	assert.Equal(t, "/home/someone/secrets.kdbx", s.KeePassXCDatabase, "database")
	assert.Equal(t, "/home/someone/secrets.key", s.KeePassXCKeyFile, "key file")
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
