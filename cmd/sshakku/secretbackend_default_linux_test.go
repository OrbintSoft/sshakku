package main

import (
	"testing"
	"time"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys"
	"github.com/OrbintSoft/sshakku/internal/secretservice"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.Truef(t, ok, "a bus that cannot be reached must fall back rather than fail outright, got %T", backend)
	assert.Equal(t, "alice", tool.User, "scoped to the account it is being opened for")
}

// TestTheSecretServiceIsGivenBothConfiguredBudgets verifies F21 where the
// wallet Linux provides is wired up: the promise is that how long to wait is
// configurable, separately for something answering on its own and something
// waiting on you, and the Secret Service does both kinds of waiting — an
// ordinary call the daemon answers itself, and a prompt the desktop puts in
// front of the user to unlock their wallet.
//
// The two budgets are given deliberately different values, so which field
// receives which says what the code believes each wait is.
func TestTheSecretServiceIsGivenBothConfiguredBudgets(t *testing.T) {
	settings := config.Settings{
		CommandTimeout:     4 * time.Second,
		InteractiveTimeout: 90 * time.Second,
	}

	client := secretServiceBudgets(&secretservice.Client{}, settings)

	assert.Equal(t, settings.CommandTimeout, client.CallTimeout,
		"an ordinary call is answered by the daemon itself, on the budget for that")
	assert.Equal(t, settings.InteractiveTimeout, client.PromptTimeout,
		"the unlock dialog is waiting on a person, and gets the budget for that instead")
}
