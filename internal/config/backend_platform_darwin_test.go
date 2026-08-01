//go:build !linux

package config

import (
	"strings"
	"testing"
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
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
		if s.SecretBackend != SecretBackendKeychain {
			t.Errorf("SecretBackend = %q, want %q", s.SecretBackend, SecretBackendKeychain)
		}
	})

	t.Run("the secret service is not a wallet this system has", func(t *testing.T) {
		s, errs := Resolve(File{SecretBackend: ptr("secret-service")}, lookupFrom(nil))

		if len(errs) == 0 {
			t.Fatal("naming a wallet this platform has not got must be reported, not silently accepted")
		}
		if got := errs[0].Error(); !strings.Contains(got, "secret-service") {
			t.Errorf("error = %q, want it to name the value that cannot be used here", got)
		}
		if s.SecretBackend != SecretBackendKeychain {
			t.Errorf("SecretBackend = %q, want the platform default %q", s.SecretBackend, SecretBackendKeychain)
		}
	})
}
