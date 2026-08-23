//go:build windows

package backend

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys/wallet"
)

// TestNewSecretBackendDefaultCredentialManager covers what a user who has
// configured no wallet gets here (F54): the store this system keeps itself.
// The name is taken from the configuration layer rather than written out,
// because which wallet is the default here is that layer's answer to give.
func TestNewSecretBackendDefaultCredentialManager(t *testing.T) {
	backend, closeFn := Open(t.Context(), "alice", fakeLogger{}, config.Settings{SecretBackend: config.DefaultSecretBackend()})
	defer closeFn()

	_, ok := backend.(*wallet.CredentialManager)
	require.Truef(t, ok, "an unconfigured machine here opens the system's own credential store, got %T", backend)
}

// TestTheStoreCarriesTheNameThisConfigurationGaveIt is F32 at this seam: the
// prefix a user chose has to reach the wallet, or a passphrase is saved under
// one name and looked for under another.
func TestTheStoreCarriesTheNameThisConfigurationGaveIt(t *testing.T) {
	settings := config.Settings{SecretBackend: config.DefaultSecretBackend(), ServicePrefix: "work"}

	backend, closeFn := Open(t.Context(), "alice", fakeLogger{}, settings)
	defer closeFn()

	store, ok := backend.(*wallet.CredentialManager)
	require.Truef(t, ok, "expected the system's own credential store, got %T", backend)
	assert.Equal(t, "work", store.ServicePrefix)
}

// TestTheStoreIsGivenTheBudgetForSomethingThatAnswersByItself is the other
// half of what F21 promises: the two budgets are separate, and this store is
// on the mechanical side of that line. It asks nobody anything — there is no
// dialog it can raise and no approval it can wait for — so waiting the length
// of a human decision on it would turn a wedged system service into a shell
// that hangs for two minutes.
//
// The two budgets are given deliberately different values, so the one handed
// over says which of them the code believes this is.
func TestTheStoreIsGivenTheBudgetForSomethingThatAnswersByItself(t *testing.T) {
	settings := config.Settings{
		SecretBackend:      config.DefaultSecretBackend(),
		CommandTimeout:     3 * time.Second,
		InteractiveTimeout: 90 * time.Second,
	}

	backend, closeFn := Open(t.Context(), "alice", fakeLogger{}, settings)
	defer closeFn()

	store, ok := backend.(*wallet.CredentialManager)
	require.Truef(t, ok, "expected the system's own credential store, got %T", backend)
	assert.Equalf(t, settings.CommandTimeout, store.Timeout,
		"this store waits on nobody, so it gets the mechanical budget and not the %s kept for a wait on a person",
		settings.InteractiveTimeout)
}
