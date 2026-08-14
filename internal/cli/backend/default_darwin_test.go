//go:build darwin

package backend

import (
	"testing"
	"time"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys/wallet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewSecretBackendDefaultKeychain covers what a user who has configured no
// wallet gets off Linux: the OS keychain, scoped to the caller's account. The
// name is taken from the configuration layer rather than written out, because
// which wallet is the default here is that layer's answer to give.
func TestNewSecretBackendDefaultKeychain(t *testing.T) {
	backend, closeFn := Open(t.Context(), "alice", fakeLogger{}, config.Settings{SecretBackend: config.DefaultSecretBackend()})
	defer closeFn()
	kc, ok := backend.(*wallet.Keychain)
	require.Truef(t, ok, "an unconfigured machine off Linux opens the OS keychain, got %T", backend)
	assert.Equal(t, "alice", kc.Account, "scoped to the account it is being opened for")
}

// TestTheKeychainIsGivenTheBudgetForAWaitOnAPerson verifies F21 where it names
// this case itself: a keychain waiting for someone to approve an access. The
// promise is not merely that such a wait is bounded — it is that how long to
// wait is configurable *separately* for something answering on its own and
// something waiting on you, and a keychain that has raised its approval dialog
// is waiting on you.
//
// The two budgets are given deliberately different values here, so the one
// handed over says which of the two the code believes this is. Reaching the
// wallet in-process rather than by running a command changes nothing about who
// is being waited for.
func TestTheKeychainIsGivenTheBudgetForAWaitOnAPerson(t *testing.T) {
	settings := config.Settings{
		SecretBackend:      config.DefaultSecretBackend(),
		CommandTimeout:     3 * time.Second,
		InteractiveTimeout: 90 * time.Second,
	}

	backend, closeFn := Open(t.Context(), "alice", fakeLogger{}, settings)
	defer closeFn()

	kc, ok := backend.(*wallet.Keychain)
	require.Truef(t, ok, "an unconfigured machine off Linux opens the OS keychain, got %T", backend)
	assert.Equalf(t, settings.InteractiveTimeout, kc.Timeout,
		"a keychain showing its approval dialog is waiting on a person, not on the %s given to something that answers by itself",
		settings.CommandTimeout)
}
