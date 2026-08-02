package keys

import "github.com/godbus/dbus/v5"

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
//
// It also implements SecretSession: a caller that wants to batch several
// Lookup/Store calls under one unlock (Loader, across a run's worth of keys)
// can call Unlock/Lock explicitly, which suppresses the per-call unlock/lock
// below until Lock is called.
type SecretServiceBackend struct {
	Client SecretServiceClient
	// User is the "username" attribute, constant for the login session.
	User string
	// Container is the collection to keep entries in, used as both its alias
	// and its label. Empty selects the collection SSHakku makes for itself.
	//
	// Whatever it names is treated as SSHakku's own: List reports every item in
	// it and forget --all deletes them, without reading whose entry is whose.
	// It must therefore never be pointed at a collection SSHakku did not make —
	// resolving one can adopt an existing collection rather than create it.
	Container string

	collection dbus.ObjectPath
	// held is true between an explicit Unlock and its matching Lock: while
	// true, Lookup/Store/Delete skip their own unlock/lock bracket.
	held bool
}

// Unlock unlocks the sshakku collection and keeps it unlocked for subsequent
// Lookup/Store/Delete calls until Lock is called.
func (b *SecretServiceBackend) Unlock() error {
	col, err := b.resolveCollection()
	if err != nil {
		return err
	}
	if err := b.Client.Unlock(col); err != nil {
		return err
	}
	b.held = true
	return nil
}

// Lock re-locks the sshakku collection previously unlocked via Unlock.
func (b *SecretServiceBackend) Lock() error {
	col, err := b.resolveCollection()
	if err != nil {
		return err
	}
	b.held = false
	return b.Client.Lock(col)
}

var _ SecretSession = (*SecretServiceBackend)(nil)

// collectionNames is how the collection is addressed: the configured name,
// else the one SSHakku makes for itself. Both are returned because a service
// that supports collection aliases resolves the alias, while one that does not
// is matched on the label — a name applied to only one of them would land in
// the configured collection on one desktop and in SSHakku's own on another.
func (b *SecretServiceBackend) collectionNames() (alias, label string) {
	if b.Container != "" {
		return b.Container, b.Container
	}
	return secretServiceAlias, secretServiceLabel
}

func (b *SecretServiceBackend) resolveCollection() (dbus.ObjectPath, error) {
	if b.collection == "" {
		alias, label := b.collectionNames()
		col, err := b.Client.Collection(alias, label)
		if err != nil {
			return "", err
		}
		b.collection = col
	}
	return b.collection, nil
}

// Lookup unlocks the sshakku collection, searches it for service, reads the
// secret if found, and re-locks the collection before returning — on a hit, a
// miss, or an error alike. When the collection is already held unlocked (see
// SecretSession), it skips its own unlock/lock and leaves that to the holder.
func (b *SecretServiceBackend) Lookup(service string) (string, bool, error) {
	col, err := b.resolveCollection()
	if err != nil {
		return "", false, err
	}
	if !b.held {
		if err := b.Client.Unlock(col); err != nil {
			return "", false, err
		}
		defer func() { _ = b.Client.Lock(col) }()
	}

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
// error alike. When the collection is already held unlocked (see
// SecretSession), it skips its own unlock/lock and leaves that to the holder.
func (b *SecretServiceBackend) Store(service, label, passphrase string) error {
	col, err := b.resolveCollection()
	if err != nil {
		return err
	}
	if !b.held {
		if err := b.Client.Unlock(col); err != nil {
			return err
		}
		defer func() { _ = b.Client.Lock(col) }()
	}

	attrs := map[string]string{"service": service, "username": b.User}
	return b.Client.CreateItem(col, label, attrs, passphrase, true)
}

// Delete unlocks the sshakku collection, deletes every item matching service,
// and re-locks the collection before returning — on success, a miss, or an
// error alike. A miss (nothing to delete) is success, not an error. When the
// collection is already held unlocked (see SecretSession), it skips its own
// unlock/lock and leaves that to the holder.
func (b *SecretServiceBackend) Delete(service string) error {
	col, err := b.resolveCollection()
	if err != nil {
		return err
	}
	if !b.held {
		if err := b.Client.Unlock(col); err != nil {
			return err
		}
		defer func() { _ = b.Client.Lock(col) }()
	}

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
// collection is dedicated to sshakku, every item in it is sshakku-managed.
// When the collection is already held unlocked (see
// SecretSession), it skips its own unlock/lock and leaves that to the holder.
func (b *SecretServiceBackend) List() ([]string, error) {
	col, err := b.resolveCollection()
	if err != nil {
		return nil, err
	}
	if !b.held {
		if err := b.Client.Unlock(col); err != nil {
			return nil, err
		}
		defer func() { _ = b.Client.Lock(col) }()
	}

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
