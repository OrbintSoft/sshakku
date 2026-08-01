//go:build darwin

package main

import (
	"testing"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys"
)

// TestNewSecretBackendDefaultKeychain covers what a user who has configured no
// wallet gets off Linux: the OS keychain, scoped to the caller's account. The
// name is taken from the configuration layer rather than written out, because
// which wallet is the default here is that layer's answer to give.
func TestNewSecretBackendDefaultKeychain(t *testing.T) {
	backend, closeFn := newSecretBackend("alice", fakeLogger{}, config.Settings{SecretBackend: config.DefaultSecretBackend()})
	defer closeFn()
	kc, ok := backend.(*keys.KeychainBackend)
	if !ok {
		t.Fatalf("backend = %T, want a *keys.KeychainBackend default off Linux", backend)
	}
	if kc.Account != "alice" {
		t.Errorf("Account = %q, want %q", kc.Account, "alice")
	}
}
