package wallet

import (
	"context"

	"github.com/godbus/dbus/v5"
)

// The collection SSHakku makes for itself, addressed by both alias and label,
// when the caller names none of its own.
const (
	secretServiceAlias = "sshakku"
	secretServiceLabel = "sshakku"
)

// The attribute keys an entry is filed under. Storing writes them and every
// lookup, delete and enumeration searches by them, so an entry written under
// one spelling and searched for under another is one nothing finds again.
const (
	secretAttrService  = "service"
	secretAttrUsername = "username"
)

// SecretServiceClient is the subset of the freedesktop Secret Service D-Bus
// API SecretService needs; *secretservice.Client implements it. Kept
// as an interface here so the backend is unit-testable without a real D-Bus
// session bus.
type SecretServiceClient interface {
	// Collection resolves (creating if necessary) the object path of the
	// collection identified by alias.
	Collection(ctx context.Context, alias, label string) (dbus.ObjectPath, error)
	Unlock(ctx context.Context, objects ...dbus.ObjectPath) error
	Lock(ctx context.Context, objects ...dbus.ObjectPath) error
	SearchItems(ctx context.Context, collection dbus.ObjectPath, attrs map[string]string) ([]dbus.ObjectPath, error)
	GetSecret(ctx context.Context, item dbus.ObjectPath) (string, error)
	CreateItem(ctx context.Context, collection dbus.ObjectPath, label string, attrs map[string]string, passphrase string, replace bool) error
	// Items returns every item currently in collection, regardless of attributes.
	Items(ctx context.Context, collection dbus.ObjectPath) ([]dbus.ObjectPath, error)
	// ItemAttributes returns the lookup attributes item was stored under.
	ItemAttributes(ctx context.Context, item dbus.ObjectPath) (map[string]string, error)
	DeleteItem(ctx context.Context, item dbus.ObjectPath) error
}

// SecretService keeps passphrases in a dedicated Secret Service
// collection, unlocking it only for the duration of each Lookup/Store and
// locking it again immediately after — instead of relying on the desktop's
// fixed idle timeout to bound the exposure window. Unlike SecretTool,
// which only ever targets the default collection and has no lock/unlock
// verbs, this talks to the Secret Service D-Bus API directly.
//
// It also implements Session: a caller that wants to batch several
// Lookup/Store calls under one unlock (Loader, across a run's worth of keys)
// can call Unlock/Lock explicitly, which suppresses the per-call unlock/lock
// below until Lock is called.
type SecretService struct {
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

// Unlock unlocks SSHakku's collection and keeps it unlocked for subsequent
// Lookup/Store/Delete calls until Lock is called.
func (b *SecretService) Unlock(ctx context.Context) error {
	col, err := b.resolveCollection(ctx)
	if err != nil {
		return err
	}
	if err := b.Client.Unlock(ctx, col); err != nil {
		return err
	}
	b.held = true
	return nil
}

// Lock re-locks SSHakku's collection previously unlocked via Unlock.
func (b *SecretService) Lock(ctx context.Context) error {
	col, err := b.resolveCollection(ctx)
	if err != nil {
		return err
	}
	b.held = false
	return b.Client.Lock(ctx, col)
}

var _ Session = (*SecretService)(nil)

// collectionNames is how the collection is addressed: the configured name,
// else the one SSHakku makes for itself. Both are returned because a service
// that supports collection aliases resolves the alias, while one that does not
// is matched on the label — a name applied to only one of them would land in
// the configured collection on one desktop and in SSHakku's own on another.
func (b *SecretService) collectionNames() (alias, label string) {
	return SecretServiceCollectionNames(b.Container)
}

// SecretServiceCollectionNames is how the compartment holding container's
// entries is addressed, an empty container selecting the one SSHakku makes for
// itself. Exported so that anything asking a wallet about that compartment
// without going through this backend — the doctor, looking — asks about the
// same one entries would be stored in.
func SecretServiceCollectionNames(container string) (alias, label string) {
	if container != "" {
		return container, container
	}
	return secretServiceAlias, secretServiceLabel
}

func (b *SecretService) resolveCollection(ctx context.Context) (dbus.ObjectPath, error) {
	if b.collection == "" {
		alias, label := b.collectionNames()
		col, err := b.Client.Collection(ctx, alias, label)
		if err != nil {
			return "", err
		}
		b.collection = col
	}
	return b.collection, nil
}

// Lookup unlocks SSHakku's collection, searches it for service, reads the
// secret if found, and re-locks the collection before returning — on a hit, a
// miss, or an error alike. When the collection is already held unlocked (see
// Session), it skips its own unlock/lock and leaves that to the holder.
func (b *SecretService) Lookup(ctx context.Context, service string) (string, bool, error) {
	col, err := b.resolveCollection(ctx)
	if err != nil {
		return "", false, err
	}
	if !b.held {
		if err := b.Client.Unlock(ctx, col); err != nil {
			return "", false, err
		}
		defer func() { _ = b.Client.Lock(ctx, col) }()
	}

	items, err := b.Client.SearchItems(ctx, col, map[string]string{secretAttrService: service, secretAttrUsername: b.User})
	if err != nil || len(items) == 0 {
		return "", false, err
	}
	passphrase, err := b.Client.GetSecret(ctx, items[0])
	if err != nil {
		return "", false, err
	}
	return passphrase, true, nil
}

// Store unlocks SSHakku's collection, creates or replaces the item for
// service, and re-locks the collection before returning — on success or
// error alike. When the collection is already held unlocked (see
// Session), it skips its own unlock/lock and leaves that to the holder.
func (b *SecretService) Store(ctx context.Context, service, label, passphrase string) error {
	col, err := b.resolveCollection(ctx)
	if err != nil {
		return err
	}
	if !b.held {
		if err := b.Client.Unlock(ctx, col); err != nil {
			return err
		}
		defer func() { _ = b.Client.Lock(ctx, col) }()
	}

	attrs := map[string]string{secretAttrService: service, secretAttrUsername: b.User}
	return b.Client.CreateItem(ctx, col, label, attrs, passphrase, true)
}

// Delete unlocks SSHakku's collection, deletes every item matching service,
// and re-locks the collection before returning — on success, a miss, or an
// error alike. A miss (nothing to delete) is success, not an error. When the
// collection is already held unlocked (see Session), it skips its own
// unlock/lock and leaves that to the holder.
func (b *SecretService) Delete(ctx context.Context, service string) error {
	col, err := b.resolveCollection(ctx)
	if err != nil {
		return err
	}
	if !b.held {
		if err := b.Client.Unlock(ctx, col); err != nil {
			return err
		}
		defer func() { _ = b.Client.Lock(ctx, col) }()
	}

	items, err := b.Client.SearchItems(ctx, col, map[string]string{secretAttrService: service, secretAttrUsername: b.User})
	if err != nil {
		return err
	}
	for _, item := range items {
		if err := b.Client.DeleteItem(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

// List unlocks SSHakku's collection, reads the "service" attribute of every
// item it holds, and re-locks the collection before returning. Every item in it
// counts as sshakku-managed, because the collection is sshakku's own — which is
// exactly what Container is trusted not to break.
// When the collection is already held unlocked (see
// Session), it skips its own unlock/lock and leaves that to the holder.
func (b *SecretService) List(ctx context.Context) ([]string, error) {
	col, err := b.resolveCollection(ctx)
	if err != nil {
		return nil, err
	}
	if !b.held {
		if err := b.Client.Unlock(ctx, col); err != nil {
			return nil, err
		}
		defer func() { _ = b.Client.Lock(ctx, col) }()
	}

	items, err := b.Client.Items(ctx, col)
	if err != nil {
		return nil, err
	}
	services := make([]string, 0, len(items))
	for _, item := range items {
		attrs, err := b.Client.ItemAttributes(ctx, item)
		if err != nil {
			return nil, err
		}
		if service := attrs[secretAttrService]; service != "" {
			services = append(services, service)
		}
	}
	return services, nil
}

var _ Backend = (*SecretService)(nil)
