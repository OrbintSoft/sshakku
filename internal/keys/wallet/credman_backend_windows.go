//go:build windows

package wallet

import (
	"context"
	"fmt"
	"time"

	"github.com/OrbintSoft/sshakku/internal/run"
)

// credentialStore is the account's credential store as this backend needs it.
// The calls behind it are this system's own and cannot be stood in for, so the
// interface is where a test replaces them — upstream of every decision this
// file makes, and never one of them.
type credentialStore interface {
	Read(target string) (credential, bool, error)
	Write(entry credential) error
	Delete(target string) (bool, error)
	List(prefix string) ([]string, error)
}

// systemCredentialStore is the store itself.
type systemCredentialStore struct{}

func (systemCredentialStore) Read(target string) (credential, bool, error) { return credRead(target) }

func (systemCredentialStore) Write(entry credential) error { return credWrite(entry) }

func (systemCredentialStore) Delete(target string) (bool, error) { return credDelete(target) }

func (systemCredentialStore) List(prefix string) ([]string, error) { return credList(prefix) }

// CredentialManager keeps passphrases in the credential store this system
// provides, one generic credential per service, filed under the service's own
// name. Like the keychain and unlike the CLI-backed wallets, no child process
// is involved: the passphrase crosses into a system call and never into
// another program's argv or standard input.
//
// It implements Backend and deliberately not Session. Session exists for a
// store that can be held open across a batch instead of locking again after
// each entry, and this one is never locked to begin with: what guards an entry
// is the account being signed in, so there is nothing to unlock and nothing
// that could be left unlocked.
type CredentialManager struct {
	// ServicePrefix is the name sshakku's own entries carry in a store shared
	// with every other program on the machine, and so is what List asks for.
	// It must be the prefix the entries were written under; zero selects the
	// default.
	ServicePrefix string

	// Timeout bounds each call into the store. Nothing here waits on a person
	// — this store asks nobody anything — so the budget is the mechanical one;
	// zero selects run.DefaultCommandTimeout. What elapses ends the waiting and
	// not the call, which has left Go for a system library where nothing can
	// reach it.
	Timeout time.Duration

	// credentials is the store these calls go to. Nil is the real one, so the
	// zero value is usable and only a test has to say otherwise.
	credentials credentialStore
}

// store resolves the backing store, defaulting to this system's own.
func (b *CredentialManager) store() credentialStore {
	if b.credentials == nil {
		return systemCredentialStore{}
	}
	return b.credentials
}

// prefix is the name this backend's entries carry, resolved once so a write
// and the list that later finds it cannot disagree.
func (b *CredentialManager) prefix() string {
	return ServicePrefixOrDefault(b.ServicePrefix)
}

// bounded runs one call into the store under b's budget. Every entry into the
// system library goes through here, so none is left unbounded by being
// forgotten.
func bounded[T any](b *CredentialManager, call func() (T, error)) (T, error) {
	timeout := b.Timeout
	if timeout <= 0 {
		timeout = run.DefaultCommandTimeout
	}
	return run.WithDeadline("the credential store", timeout, call)
}

// credentialLookup is what a bounded read answers with, the two halves carried
// together so one call can hand back both.
type credentialLookup struct {
	secret string
	found  bool
}

// Lookup reads the passphrase filed under service. A service nothing was ever
// stored for is a miss rather than an error.
func (b *CredentialManager) Lookup(_ context.Context, service string) (string, bool, error) {
	got, err := bounded(b, func() (credentialLookup, error) {
		entry, found, err := b.store().Read(service)
		return credentialLookup{secret: entry.Secret, found: found}, err
	})
	return got.secret, got.found, err
}

// Store files passphrase under service, replacing whatever was there. The
// store overwrites in place, so unlike the wallets whose CLI has no in-place
// edit this is one call and not a delete followed by a write.
//
// label is what a person reads beside the entry; the prefix goes in as the
// entry's user name, which is what this system's own tooling shows in the
// column beside the target — so an entry answers "who put this here" where it
// is looked at, and not only in a name a reader has to know how to parse.
func (b *CredentialManager) Store(_ context.Context, service, label, passphrase string) error {
	_, err := bounded(b, func() (struct{}, error) {
		return struct{}{}, b.store().Write(credential{
			Target:  service,
			Comment: label,
			User:    b.prefix(),
			Secret:  passphrase,
		})
	})
	return err
}

// Delete removes the entry for service. An entry that was already gone is
// success: forgetting an already-forgotten key is idempotent.
func (b *CredentialManager) Delete(_ context.Context, service string) error {
	_, err := bounded(b, func() (bool, error) { return b.store().Delete(service) })
	return err
}

// List returns the service identifiers of every entry sshakku has stored here.
//
// The store answers a prefix query itself, so the narrowing happens in the
// question and the answer is taken whole. Nothing filters it a second time:
// a second filter downstream would keep working if the query ever stopped
// being narrow, which is precisely the failure it would need to reveal — and
// what is at stake is a store shared with every other program on the machine.
func (b *CredentialManager) List(_ context.Context) ([]string, error) {
	names, err := bounded(b, func() ([]string, error) { return b.store().List(b.prefix() + "-") })
	if err != nil {
		return nil, fmt.Errorf("list stored passphrases: %w", err)
	}
	return names, nil
}
