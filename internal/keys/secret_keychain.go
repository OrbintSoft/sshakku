package keys

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
}

// Lookup reads the passphrase for service via Client.Find.
func (b *KeychainBackend) Lookup(service string) (string, bool, error) {
	return b.Client.Find(b.Account, service)
}

// Store creates the item for service if absent, or overwrites its passphrase
// in place if one already exists. Unlike OnePasswordBackend (whose CLI has
// no in-place edit), Security.framework supports updating an item directly,
// so Store checks for an existing item first rather than deleting and
// recreating it.
func (b *KeychainBackend) Store(service, label, passphrase string) error {
	_, found, err := b.Client.Find(b.Account, service)
	if err != nil {
		return err
	}
	if found {
		return b.Client.Update(b.Account, service, passphrase)
	}
	return b.Client.Add(b.Account, service, label, passphrase)
}

// Delete removes the item for service. A missing entry is success, not an
// error — deleting an already-forgotten key is idempotent.
func (b *KeychainBackend) Delete(service string) error {
	return b.Client.Delete(b.Account, service)
}

// List returns the service identifiers of every item stored under Account.
func (b *KeychainBackend) List() ([]string, error) {
	return b.Client.List(b.Account)
}

var _ SecretBackend = (*KeychainBackend)(nil)
