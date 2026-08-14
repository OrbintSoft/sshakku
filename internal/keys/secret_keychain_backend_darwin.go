//go:build darwin

package keys

import (
	"context"
	"time"

	"github.com/OrbintSoft/sshakku/internal/run"
)

// KeychainClient is the subset of macOS's Security.framework generic-password
// API KeychainBackend needs; the darwin build provides the real
// implementation over cgo. Kept as an interface here so the backend is
// unit-testable without a real keychain.
type KeychainClient interface {
	// Add creates a new generic-password item for account/service, labeled
	// label, holding passphrase.
	Add(account, service, label, passphrase string) error
	// Update overwrites the passphrase of an existing item for
	// account/service.
	Update(account, service, passphrase string) error
	// Find returns the passphrase for account/service and whether an item
	// was found. A miss is reported as found=false, not an error.
	Find(account, service string) (passphrase string, found bool, err error)
	// Delete removes the item for account/service. A missing entry is
	// success, not an error.
	Delete(account, service string) error
	// List returns the service identifiers of every item stored under
	// account.
	List(account string) ([]string, error)
}

// KeychainBackend keeps passphrases in the macOS keychain as generic-password
// items, one per service, all under the same account. Unlike
// SecretToolBackend/OnePasswordBackend it never shells out to a child
// process: the passphrase only ever crosses into Client's Security.framework
// calls, never a subprocess's argv or stdin.
type KeychainBackend struct {
	Client KeychainClient
	// Account is the "account" attribute every item is stored under,
	// constant for the login session.
	Account string
	// Timeout bounds each keychain call. The framework can wait on an
	// authorization nobody is there to grant, and something is waiting on the
	// answer — a login shell, or an ssh at a passphrase prompt — so the wait is
	// finite and the caller falls back to asking on the terminal. Zero selects
	// run.DefaultInteractiveTimeout: when the framework does put up its approval
	// dialog, what this call is waiting for is a person deciding, and from out
	// here that call looks no different from one that answers by itself.
	//
	// Note what it does not do: a call already inside the framework cannot be
	// cancelled from Go, so what elapses here ends the waiting, not the call.
	Timeout time.Duration

	// ServicePrefix is the name sshakku's own items carry in the login
	// keychain, and so is what List goes by when deciding which items are
	// sshakku's to report. It must be the prefix the entries were written
	// under; zero selects the default.
	ServicePrefix string
}

// keychainSecret is what a lookup answers with, the two halves carried together
// so a bounded call can hand back both.
type keychainSecret struct {
	passphrase string
	found      bool
}

// bounded runs one keychain call under b's budget. Every entry into the
// framework goes through here or through boundedErr, so no call is left
// unbounded by being forgotten.
func bounded[T any](b *KeychainBackend, call func() (T, error)) (T, error) {
	timeout := b.Timeout
	if timeout <= 0 {
		timeout = run.DefaultInteractiveTimeout
	}
	return run.WithDeadline("the keychain", timeout, call)
}

// boundedErr runs one keychain call that answers with nothing but an error.
func boundedErr(b *KeychainBackend, call func() error) error {
	_, err := bounded(b, func() (struct{}, error) { return struct{}{}, call() })
	return err
}

// find reads the item for service, bounded.
func (b *KeychainBackend) find(service string) (keychainSecret, error) {
	return bounded(b, func() (keychainSecret, error) {
		passphrase, found, err := b.Client.Find(b.Account, service)
		return keychainSecret{passphrase: passphrase, found: found}, err
	})
}

// Lookup reads the passphrase for service via Client.Find.
func (b *KeychainBackend) Lookup(ctx context.Context, service string) (string, bool, error) {
	got, err := b.find(service)
	return got.passphrase, got.found, err
}

// Store creates the item for service if absent, or overwrites its passphrase
// in place if one already exists. Unlike OnePasswordBackend (whose CLI has
// no in-place edit), Security.framework supports updating an item directly,
// so Store checks for an existing item first rather than deleting and
// recreating it.
//
// That makes it two calls, and each gets the budget in full: the budget bounds
// a call into the framework, and there is no way to hand one the remainder of
// another's.
func (b *KeychainBackend) Store(ctx context.Context, service, label, passphrase string) error {
	existing, err := b.find(service)
	if err != nil {
		return err
	}
	if existing.found {
		return boundedErr(b, func() error { return b.Client.Update(b.Account, service, passphrase) })
	}
	return boundedErr(b, func() error { return b.Client.Add(b.Account, service, label, passphrase) })
}

// Delete removes the item for service. A missing entry is success, not an
// error — deleting an already-forgotten key is idempotent.
func (b *KeychainBackend) Delete(ctx context.Context, service string) error {
	return boundedErr(b, func() error { return b.Client.Delete(b.Account, service) })
}

// List returns the service identifiers of the items sshakku stored under
// Account. A generic-password query can be narrowed by account but not by any
// prefix of the service, and the login keychain is where every application on
// the machine keeps its passwords — so the client answers with all of them and
// the rest is dropped here. Another program's secret is not sshakku's to
// report, let alone to hand to `forget --all` (F27).
func (b *KeychainBackend) List(ctx context.Context) ([]string, error) {
	services, err := bounded(b, func() ([]string, error) { return b.Client.List(b.Account) })
	if err != nil {
		return nil, err
	}
	return ownServices(services, b.ServicePrefix), nil
}

var _ SecretBackend = (*KeychainBackend)(nil)
