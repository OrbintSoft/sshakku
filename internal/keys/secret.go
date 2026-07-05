package keys

import (
	"fmt"
	"strings"

	"github.com/godbus/dbus/v5"
)

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

var _ SecretBackend = (*SecretServiceBackend)(nil)
