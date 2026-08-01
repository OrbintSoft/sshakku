//go:build linux

package config

import (
	"strings"
	"testing"
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
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
		if s.SecretBackend != SecretBackendSecretService {
			t.Errorf("SecretBackend = %q, want %q", s.SecretBackend, SecretBackendSecretService)
		}
	})

	t.Run("the keychain is not a wallet this system has", func(t *testing.T) {
		s, errs := Resolve(File{SecretBackend: ptr("keychain")}, lookupFrom(nil))

		if len(errs) == 0 {
			t.Fatal("naming a wallet this platform has not got must be reported, not silently accepted")
		}
		if got := errs[0].Error(); !strings.Contains(got, "keychain") {
			t.Errorf("error = %q, want it to name the value that cannot be used here", got)
		}
		if s.SecretBackend != SecretBackendSecretService {
			t.Errorf("SecretBackend = %q, want the platform default %q", s.SecretBackend, SecretBackendSecretService)
		}
	})
}
