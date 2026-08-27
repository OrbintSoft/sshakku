//go:build windows

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSecretBackendChoicesOnWindows verifies F26 and F54 here: this system has
// a wallet of its own, it is what an unnamed backend gets, and a wallet
// belonging to another platform is a mistake in the configuration rather than
// a piece to go and install.
//
// The names are written out rather than taken from constants, as the macOS
// test writes its own out: what a user puts in a configuration file is a
// string, and this is the string they typed.
func TestSecretBackendChoicesOnWindows(t *testing.T) {
	t.Run("named nothing, gets the store this system keeps", func(t *testing.T) {
		s, errs := Resolve(File{}, lookupFrom(nil))
		require.Empty(t, errs, "unexpected errors")
		assert.Equal(t, "credential-manager", s.SecretBackend, "SecretBackend")
	})

	t.Run("the secret service is not a wallet this system has", func(t *testing.T) {
		s, errs := Resolve(File{SecretBackend: new("secret-service")}, lookupFrom(nil))

		require.NotEmpty(t, errs, "naming a wallet this platform has not got must be reported, not silently accepted")
		assert.Contains(t, errs[0].Error(), "secret-service", "the error must name the value that cannot be used here")
		assert.Equal(t, "credential-manager", s.SecretBackend, "SecretBackend must fall back to the platform default")
	})

	t.Run("the store this system keeps can be named outright", func(t *testing.T) {
		s, errs := Resolve(File{SecretBackend: new("credential-manager")}, lookupFrom(nil))
		require.Empty(t, errs, "unexpected errors")
		assert.Equal(t, "credential-manager", s.SecretBackend, "SecretBackend")
	})

	// F56: the wallet is named in the configuration and that is the whole of
	// what naming it takes here — no setting of this platform's own, the way
	// KeePassXC needs a database named.
	t.Run("1Password can be named outright", func(t *testing.T) {
		assert.Contains(t, SecretBackends(), "1password", "the wallet must be one this platform offers")

		s, errs := Resolve(File{SecretBackend: new("1password")}, lookupFrom(nil))
		require.Empty(t, errs, "unexpected errors")
		assert.Equal(t, "1password", s.SecretBackend, "SecretBackend")
	})

	// Bitwarden is reached by running someone else's program, as 1Password is,
	// and is still not offered here. It compiles on this platform, which is not
	// the same as having been shown to work on it, and each one offered is a
	// promise the test matrix owes a row for.
	t.Run("a wallet nobody has driven here is not offered", func(t *testing.T) {
		assert.NotContains(t, SecretBackends(), "bitwarden")
	})
}
