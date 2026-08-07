//go:build darwin

package main

import (
	"testing"
	"time"

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

	backend, closeFn := newSecretBackend("alice", fakeLogger{}, settings)
	defer closeFn()

	kc, ok := backend.(*keys.KeychainBackend)
	if !ok {
		t.Fatalf("backend = %T, want a *keys.KeychainBackend default off Linux", backend)
	}
	if kc.Timeout != settings.InteractiveTimeout {
		t.Errorf("Timeout = %s, want %s — the budget for a wait on a person, not the %s given to something that answers on its own",
			kc.Timeout, settings.InteractiveTimeout, settings.CommandTimeout)
	}
}
