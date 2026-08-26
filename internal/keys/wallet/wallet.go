package wallet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// jsonMarshal is the item-template encoder the shell-out backends (Bitwarden,
// 1Password) use to build their stdin payloads. It is a seam so the otherwise
// unreachable marshal-failure branch of each Store can be exercised.
var jsonMarshal = json.Marshal

// exitError is an external wallet program that ran and refused. Every
// shell-out backend here reports a refusal the same way — the command as the
// reader should see it named, the status it exited with, and what it said on
// its error stream — and one type saying it once means a caller can read the
// status back instead of matching on the sentence.
type exitError struct {
	command string
	code    int
	stderr  string
}

func (e exitError) Error() string {
	return fmt.Sprintf("%s exited %d: %s", e.command, e.code, e.stderr)
}

// ErrListUnsupported is returned by List on a backend that cannot enumerate
// its stored entries — e.g. SecretTool, which has no generic
// "list everything" verb without already knowing exact attributes.
var ErrListUnsupported = errors.New("secret backend does not support listing stored entries")

// ownServices keeps only the entries sshakku itself stored, dropping anything
// else the store holds. Every service string sshakku generates begins with
// prefix, so that prefix is what tells its own entries from the rest.
//
// prefix is an argument rather than a constant read here because the same
// prefix decides what a store writes: were the two to disagree, everything
// written would be someone else's to this function, and forgetting everything
// would forget nothing. An empty prefix selects the default, so a caller that
// never set one behaves as before.
//
// It is for the backends whose store is shared with other programs and whose
// enumeration cannot be narrowed at the query — the macOS login keychain, which
// answers for every application that ever saved a password under this account,
// and a Bitwarden vault, which `bw list items` returns whole. A backend that
// already asks only for its own entries (a dedicated collection, a vault tag, a
// database group) must not filter again: the query is where that guarantee
// belongs, and a second filter downstream would hide its absence.
func ownServices(names []string, prefix string) []string {
	own := make([]string, 0, len(names))
	for _, name := range names {
		if strings.HasPrefix(name, ServicePrefixOrDefault(prefix)+"-") {
			own = append(own, name)
		}
	}
	return own
}

// Backend stores and retrieves a key's passphrase in the OS secret store.
// It is the seam the platform secret stores plug into — the D-Bus Secret Service
// and its secret-tool fallback on Linux, the OS keychain off it, plus the
// CLI-backed 1Password and Bitwarden backends. service is an opaque per-key
// identifier the backend maps onto its own schema.
//
// Every call takes the caller's context. What is behind this seam is another
// program or another process — a session-bus daemon, a vault's CLI, a socket —
// and each of them can be slow, locked, or waiting on a person, so the caller
// that can afford to stop waiting is the one holding the context.
type Backend interface {
	// Lookup returns the stored passphrase for service and whether one was found.
	// A miss is reported as found=false, not an error.
	Lookup(ctx context.Context, service string) (passphrase string, found bool, err error)
	// Store saves passphrase for service under a human-readable label.
	Store(ctx context.Context, service, label, passphrase string) error
	// Delete removes the entry for service. A missing entry is success, not an
	// error — deleting an already-forgotten key is idempotent.
	Delete(ctx context.Context, service string) error
	// List returns the service identifiers of every entry sshakku manages, for
	// forgetting them all at once. Returns ErrListUnsupported if the backend
	// cannot enumerate its entries.
	List(ctx context.Context) ([]string, error)
}

// Session is implemented by backends that can hold their store unlocked
// across several Lookup/Store calls instead of locking again after each one.
// Loader uses it to unlock once for a whole batch of keys — closing the
// wallet as soon as the batch finishes rather than once per key — while a
// single reactive lookup (the askpass broker refilling one expired key)
// still gets its own short unlock/lock, unaffected by this interface.
type Session interface {
	// Unlock unlocks the store and keeps it unlocked for subsequent
	// Lookup/Store/Delete calls until Lock is called.
	Unlock(ctx context.Context) error
	// Lock re-locks the store previously unlocked via Unlock.
	Lock(ctx context.Context) error
}

// defaultServicePrefix names sshakku's own entries in a shared store, and is
// how they are told apart from everyone else's there — so it names who put the
// entry there, not what the entry holds. A prefix of the second kind would be
// one any program keeping an SSH passphrase might pick too.
const defaultServicePrefix = "SSHakku-Key"

// DefaultServicePrefix is the service prefix used when no per-config override
// is set.
const DefaultServicePrefix = defaultServicePrefix

// ServicePrefixOrDefault resolves an unset prefix to the default. It is the one
// place that decides what "unset" means, so a store that writes an entry and a
// sweep that enumerates one cannot resolve it differently.
func ServicePrefixOrDefault(prefix string) string {
	if prefix != "" {
		return prefix
	}
	return defaultServicePrefix
}
