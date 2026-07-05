package keys

import (
	"errors"
	"fmt"
	"strings"

	"github.com/godbus/dbus/v5"
)

// ErrListUnsupported is returned by List on a backend that cannot enumerate
// its stored entries — e.g. SecretToolBackend, which has no generic
// "list everything" verb without already knowing exact attributes.
var ErrListUnsupported = errors.New("secret backend does not support listing stored entries")

// SecretBackend stores and retrieves a key's passphrase in the OS secret store.
// It is the seam the platform secret stores plug into; this slice ships only the
// D-Bus Secret Service (KDE Wallet / GNOME Keyring) implementation below. service
// is an opaque per-key identifier the backend maps onto its own schema.
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

// secretToolBin is the libsecret CLI shared by KDE Wallet and GNOME Keyring,
// which both implement the D-Bus Secret Service API.
const secretToolBin = "secret-tool"

// SecretToolBackend keeps passphrases in the D-Bus Secret Service via secret-tool.
// The passphrase travels on the process's stdin, never in argv, so it cannot leak
// through `ps` or /proc/<pid>/cmdline.
type SecretToolBackend struct {
	Runner Runner
	// User is the "username" attribute, constant for the login session.
	User string
}

// Lookup runs `secret-tool lookup service <service> username <user>`. secret-tool
// emits the secret verbatim, so a trailing newline (e.g. from an entry stored by
// the earlier shell version) is trimmed. A non-zero exit means no entry — handled
// as a miss, not an error, so the loader falls back to prompting.
func (b SecretToolBackend) Lookup(service string) (string, bool, error) {
	res, err := b.Runner.Run(Cmd{
		Name: secretToolBin,
		Args: []string{"lookup", "service", service, "username", b.User},
	})
	if err != nil {
		return "", false, err
	}
	if res.Code != 0 {
		return "", false, nil
	}
	return strings.TrimRight(string(res.Stdout), "\n"), true, nil
}

// Store runs `secret-tool store --label=<label> service <service> username
// <user>`, feeding the passphrase on stdin. Unlike the earlier `echo | …`, no
// trailing newline is appended, so the secret is stored exactly.
func (b SecretToolBackend) Store(service, label, passphrase string) error {
	res, err := b.Runner.Run(Cmd{
		Name:  secretToolBin,
		Args:  []string{"store", "--label=" + label, "service", service, "username", b.User},
		Stdin: passphrase,
	})
	if err != nil {
		return err
	}
	if res.Code != 0 {
		return fmt.Errorf("secret-tool store exited %d: %s", res.Code, strings.TrimSpace(string(res.Stderr)))
	}
	return nil
}

// Delete runs `secret-tool clear service <service> username <user>`. Known
// caveat from manual testing: secret-tool clear has been observed to exit 1
// with no stderr against a real, present entry — a silent failure to remove
// it — so a non-zero exit here is reported but should not be over-trusted as
// proof the entry is gone; there is no native-D-Bus alternative on this
// fallback path (it exists only because the D-Bus session itself was
// unreachable).
func (b SecretToolBackend) Delete(service string) error {
	res, err := b.Runner.Run(Cmd{
		Name: secretToolBin,
		Args: []string{"clear", "service", service, "username", b.User},
	})
	if err != nil {
		return err
	}
	if res.Code != 0 {
		return fmt.Errorf("secret-tool clear exited %d: %s", res.Code, strings.TrimSpace(string(res.Stderr)))
	}
	return nil
}

// List always fails: secret-tool has no verb to enumerate entries without
// already knowing their exact attributes, so "forget everything" is only
// supported through SecretServiceBackend's native D-Bus enumeration.
func (b SecretToolBackend) List() ([]string, error) {
	return nil, ErrListUnsupported
}

var _ SecretBackend = SecretToolBackend{}

const (
	secretServiceAlias = "sshakku"
	secretServiceLabel = "sshakku"
)

// SecretServiceClient is the subset of the freedesktop Secret Service D-Bus
// API SecretServiceBackend needs; *secretservice.Client implements it. Kept
// as an interface here so the backend is unit-testable without a real D-Bus
// session bus.
type SecretServiceClient interface {
	// Collection resolves (creating if necessary) the object path of the
	// collection identified by alias.
	Collection(alias, label string) (dbus.ObjectPath, error)
	Unlock(objects ...dbus.ObjectPath) error
	Lock(objects ...dbus.ObjectPath) error
	SearchItems(collection dbus.ObjectPath, attrs map[string]string) ([]dbus.ObjectPath, error)
	GetSecret(item dbus.ObjectPath) (string, error)
	CreateItem(collection dbus.ObjectPath, label string, attrs map[string]string, passphrase string, replace bool) error
	// Items returns every item currently in collection, regardless of attributes.
	Items(collection dbus.ObjectPath) ([]dbus.ObjectPath, error)
	// ItemAttributes returns the lookup attributes item was stored under.
	ItemAttributes(item dbus.ObjectPath) (map[string]string, error)
	DeleteItem(item dbus.ObjectPath) error
}

// SecretServiceBackend keeps passphrases in a dedicated Secret Service
// collection, unlocking it only for the duration of each Lookup/Store and
// locking it again immediately after — instead of relying on the desktop's
// fixed idle timeout to bound the exposure window. Unlike SecretToolBackend,
// which only ever targets the default collection and has no lock/unlock
// verbs, this talks to the Secret Service D-Bus API directly.
type SecretServiceBackend struct {
	Client SecretServiceClient
	// User is the "username" attribute, constant for the login session.
	User string

	collection dbus.ObjectPath
}

func (b *SecretServiceBackend) resolveCollection() (dbus.ObjectPath, error) {
	if b.collection == "" {
		col, err := b.Client.Collection(secretServiceAlias, secretServiceLabel)
		if err != nil {
			return "", err
		}
		b.collection = col
	}
	return b.collection, nil
}

// Lookup unlocks the sshakku collection, searches it for service, reads the
// secret if found, and re-locks the collection before returning — on a hit, a
// miss, or an error alike.
func (b *SecretServiceBackend) Lookup(service string) (string, bool, error) {
	col, err := b.resolveCollection()
	if err != nil {
		return "", false, err
	}
	if err := b.Client.Unlock(col); err != nil {
		return "", false, err
	}
	defer func() { _ = b.Client.Lock(col) }()

	items, err := b.Client.SearchItems(col, map[string]string{"service": service, "username": b.User})
	if err != nil || len(items) == 0 {
		return "", false, err
	}
	passphrase, err := b.Client.GetSecret(items[0])
	if err != nil {
		return "", false, err
	}
	return passphrase, true, nil
}

// Store unlocks the sshakku collection, creates or replaces the item for
// service, and re-locks the collection before returning — on success or
// error alike.
func (b *SecretServiceBackend) Store(service, label, passphrase string) error {
	col, err := b.resolveCollection()
	if err != nil {
		return err
	}
	if err := b.Client.Unlock(col); err != nil {
		return err
	}
	defer func() { _ = b.Client.Lock(col) }()

	attrs := map[string]string{"service": service, "username": b.User}
	return b.Client.CreateItem(col, label, attrs, passphrase, true)
}

// Delete unlocks the sshakku collection, deletes every item matching service,
// and re-locks the collection before returning — on success, a miss, or an
// error alike. A miss (nothing to delete) is success, not an error.
func (b *SecretServiceBackend) Delete(service string) error {
	col, err := b.resolveCollection()
	if err != nil {
		return err
	}
	if err := b.Client.Unlock(col); err != nil {
		return err
	}
	defer func() { _ = b.Client.Lock(col) }()

	items, err := b.Client.SearchItems(col, map[string]string{"service": service, "username": b.User})
	if err != nil {
		return err
	}
	for _, item := range items {
		if err := b.Client.DeleteItem(item); err != nil {
			return err
		}
	}
	return nil
}

// List unlocks the sshakku collection, reads the "service" attribute of every
// item it holds, and re-locks the collection before returning. Since the
// collection is dedicated to sshakku (open decision 17), every item in it is
// sshakku-managed.
func (b *SecretServiceBackend) List() ([]string, error) {
	col, err := b.resolveCollection()
	if err != nil {
		return nil, err
	}
	if err := b.Client.Unlock(col); err != nil {
		return nil, err
	}
	defer func() { _ = b.Client.Lock(col) }()

	items, err := b.Client.Items(col)
	if err != nil {
		return nil, err
	}
	services := make([]string, 0, len(items))
	for _, item := range items {
		attrs, err := b.Client.ItemAttributes(item)
		if err != nil {
			return nil, err
		}
		if service := attrs["service"]; service != "" {
			services = append(services, service)
		}
	}
	return services, nil
}

var _ SecretBackend = (*SecretServiceBackend)(nil)
