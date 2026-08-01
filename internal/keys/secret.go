package keys

import (
	"encoding/json"
	"errors"
	"strings"
)

// jsonMarshal is the item-template encoder the shell-out backends (Bitwarden,
// 1Password) use to build their stdin payloads. It is a seam so the otherwise
// unreachable marshal-failure branch of each Store can be exercised.
var jsonMarshal = json.Marshal

// ErrListUnsupported is returned by List on a backend that cannot enumerate
// its stored entries — e.g. SecretToolBackend, which has no generic
// "list everything" verb without already knowing exact attributes.
var ErrListUnsupported = errors.New("secret backend does not support listing stored entries")

// ownServices keeps only the entries sshakku itself stored, dropping anything
// else the store holds. Every service string sshakku generates begins with the
// key prefix, so that prefix is what tells its own entries from the rest.
//
// It is for the backends whose store is shared with other programs and whose
// enumeration cannot be narrowed at the query — the macOS login keychain, which
// answers for every application that ever saved a password under this account,
// and a Bitwarden vault, which `bw list items` returns whole. A backend that
// already asks only for its own entries (a dedicated collection, a vault tag, a
// database group) must not filter again: the query is where that guarantee
// belongs, and a second filter downstream would hide its absence.
// The legacy prefix is kept here so an entry an older version wrote is still
// sshakku's to forget: dropping it would leave those entries where nothing
// this program offers could ever delete them.
func ownServices(names []string) []string {
	own := make([]string, 0, len(names))
	for _, name := range names {
		if strings.HasPrefix(name, defaultServicePrefix+"-") || strings.HasPrefix(name, legacyServicePrefix+"-") {
			own = append(own, name)
		}
	}
	return own
}

// legacyService is service under the name earlier versions of sshakku wrote,
// or "" when service is not one sshakku itself names — a configured prefix
// override was never written by an older version, so there is nothing there
// to fall back to.
func legacyService(service string) string {
	name, ok := strings.CutPrefix(service, defaultServicePrefix+"-")
	if !ok {
		return ""
	}
	return legacyServicePrefix + "-" + name
}

// lookupWithLegacy reads service, and when nothing is stored under it, tries
// the name earlier versions used — moving what it finds over to the current
// name and deleting the old entry.
//
// Moving it is what ends the fallback. A key whose passphrase is never asked
// for again would otherwise keep its old entry for good, and that old name
// said what the entry held rather than who had put it there, which is a name
// any program keeping an SSH passphrase might have chosen for itself.
//
// Failing to move is not failing to read: the passphrase was found and is
// returned either way, and the tidying is tried again on the next lookup.
func lookupWithLegacy(b SecretBackend, service, keyname string, logf func(level, format string, args ...any)) (string, bool, error) {
	pass, found, err := b.Lookup(service)
	if err != nil || found {
		return pass, found, err
	}

	old := legacyService(service)
	if old == "" {
		return "", false, nil
	}
	pass, found, err = b.Lookup(old)
	if err != nil || !found || strings.TrimSpace(pass) == "" {
		return "", false, err
	}

	if err := b.Store(service, "SSH Passphrase for "+keyname, pass); err != nil {
		logf("INFO", "kept reading %s under its old name: %v", keyname, err)
		return pass, true, nil
	}
	if err := b.Delete(old); err != nil {
		logf("INFO", "moved %s to its current name but could not remove the old entry: %v", keyname, err)
	}
	return pass, true, nil
}

// SecretBackend stores and retrieves a key's passphrase in the OS secret store.
// It is the seam the platform secret stores plug into — the D-Bus Secret Service
// and its secret-tool fallback on Linux, the OS keychain off it, plus the
// CLI-backed 1Password and Bitwarden backends. service is an opaque per-key
// identifier the backend maps onto its own schema.
type SecretBackend interface {
	// Lookup returns the stored passphrase for service and whether one was found.
	// A miss is reported as found=false, not an error.
	Lookup(service string) (passphrase string, found bool, err error)
	// Store saves passphrase for service under a human-readable label.
	Store(service, label, passphrase string) error
	// Delete removes the entry for service. A missing entry is success, not an
	// error — deleting an already-forgotten key is idempotent.
	Delete(service string) error
	// List returns the service identifiers of every entry sshakku manages, for
	// forgetting them all at once. Returns ErrListUnsupported if the backend
	// cannot enumerate its entries.
	List() ([]string, error)
}

// SecretSession is implemented by backends that can hold their store unlocked
// across several Lookup/Store calls instead of locking again after each one.
// Loader uses it to unlock once for a whole batch of keys — closing the
// wallet as soon as the batch finishes rather than once per key — while a
// single reactive lookup (the askpass broker refilling one expired key)
// still gets its own short unlock/lock, unaffected by this interface.
type SecretSession interface {
	// Unlock unlocks the store and keeps it unlocked for subsequent
	// Lookup/Store/Delete calls until Lock is called.
	Unlock() error
	// Lock re-locks the store previously unlocked via Unlock.
	Lock() error
}
