package main

import (
	"testing"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys"
)

// TestNewSecretBackendFallback covers the default secret-service branch's
// fallback: with the session bus pointed at an unreachable address,
// secretservice.NewClient fails and newSecretBackend hands back a
// SecretToolBackend so lookups can still go through the desktop's default
// collection. The live-bus success return needs a real D-Bus session and is
// left to integration coverage.
func TestNewSecretBackendFallback(t *testing.T) {
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/nonexistent/sshakku-no-such-bus")
	backend, closeFn := newSecretBackend("alice", fakeLogger{}, config.Settings{SecretBackend: config.SecretBackendSecretService})
	defer closeFn()
	tool, ok := backend.(keys.SecretToolBackend)
	if !ok {
		t.Fatalf("backend = %T, want a keys.SecretToolBackend fallback", backend)
	}
	if tool.User != "alice" {
		t.Errorf("User = %q, want %q", tool.User, "alice")
	}
}
