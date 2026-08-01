//go:build !linux

package main

import (
	"testing"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys"
)

// TestNewSecretBackendDefaultKeychain covers the default (secret-service)
// branch off Linux: with no D-Bus Secret Service to dial, newDefaultSecretBackend
// resolves to the OS keychain instead, scoped to the caller's account.
func TestNewSecretBackendDefaultKeychain(t *testing.T) {
	backend, closeFn := newSecretBackend("alice", fakeLogger{}, config.Settings{SecretBackend: config.SecretBackendSecretService})
	defer closeFn()
	kc, ok := backend.(*keys.KeychainBackend)
	if !ok {
		t.Fatalf("backend = %T, want a *keys.KeychainBackend default off Linux", backend)
	}
	if kc.Account != "alice" {
		t.Errorf("Account = %q, want %q", kc.Account, "alice")
	}
}
