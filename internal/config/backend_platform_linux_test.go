//go:build linux

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSecretBackendChoicesOnLinux verifies F26 for Linux: the wallet a Linux
// system has is the freedesktop Secret Service, and the macOS keychain is not
// something it can be pointed at.
//
// Naming a wallet the platform has not got is a mistake in the configuration,
// not a wallet with a missing piece, so it is reported and the platform's own
// default is used — falling back rather than failing, which is what F17
// promises for an unavailable backend.
func TestSecretBackendChoicesOnLinux(t *testing.T) {
	t.Run("named nothing, gets the secret service", func(t *testing.T) {
		s, errs := Resolve(File{}, lookupFrom(nil))
		require.Empty(t, errs, "unexpected errors")
		assert.Equal(t, SecretBackendSecretService, s.SecretBackend, "SecretBackend")
	})

	t.Run("the keychain is not a wallet this system has", func(t *testing.T) {
		s, errs := Resolve(File{SecretBackend: new("keychain")}, lookupFrom(nil))

		require.NotEmpty(t, errs, "naming a wallet this platform has not got must be reported, not silently accepted")
		assert.Contains(t, errs[0].Error(), "keychain", "the error must name the value that cannot be used here")
		assert.Equal(t, SecretBackendSecretService, s.SecretBackend, "SecretBackend must fall back to the platform default")
	})
}
