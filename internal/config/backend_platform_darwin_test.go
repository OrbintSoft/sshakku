//go:build darwin

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSecretBackendChoicesOffLinux verifies F26 where the reported defect was
// met: on macOS the wallet is the OS keychain, and the freedesktop Secret
// Service is not something the system has at all.
//
// The name is written out rather than taken from a constant, because off Linux
// there is no such constant to take it from — that is the point. What the user
// can type into a configuration file is a string, and this is the string they
// typed.
func TestSecretBackendChoicesOffLinux(t *testing.T) {
	t.Run("named nothing, gets the keychain", func(t *testing.T) {
		s, errs := Resolve(File{}, lookupFrom(nil))
		require.Empty(t, errs, "unexpected errors")
		assert.Equal(t, SecretBackendKeychain, s.SecretBackend, "SecretBackend")
	})

	t.Run("the secret service is not a wallet this system has", func(t *testing.T) {
		s, errs := Resolve(File{SecretBackend: new("secret-service")}, lookupFrom(nil))

		require.NotEmpty(t, errs, "naming a wallet this platform has not got must be reported, not silently accepted")
		assert.Contains(t, errs[0].Error(), "secret-service", "the error must name the value that cannot be used here")
		assert.Equal(t, SecretBackendKeychain, s.SecretBackend, "SecretBackend must fall back to the platform default")
	})
}
