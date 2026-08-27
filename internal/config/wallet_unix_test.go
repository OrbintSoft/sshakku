//go:build unix

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// What is left here is what this family alone can ask. The tests that were held
// here because Windows had no wallet to name have moved out as it gained them —
// the ones about KeePassXC to keepassxc_test.go, and the two about what any
// system offers to wallet_test.go, where they run everywhere.
//
// This one stays because it names Bitwarden, which Windows does not offer: a
// wallet is offered on a platform once it has been driven there, and that one
// has not been.
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
