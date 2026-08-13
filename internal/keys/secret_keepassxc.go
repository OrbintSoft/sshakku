package keys

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/OrbintSoft/sshakku/internal/keepassxc"
)

// keepassxcURLScheme is the scheme SSHakku's entries are keyed by in the user's
// database. KeePassXC's protocol only looks entries up by URL — it has no verb
// for fetching a secret by name — so a key's passphrase is stored under a URL
// built from the key's own name.
//
// The scheme is SSHakku's own so an entry cannot collide with a real site or
// host the user has saved. What must hold for KeePassXC to find it is that the
// URL yields a non-empty host, which it rejects an entry without.
const keepassxcURLScheme = "sshakku"

// keepassxcGroup is the group name sent with a new entry. KeePassXC files an
// entry by the uuid of an existing group, not by a name, so a name alone does
// not move it: entries land in the group KeePassXC keeps for entries saved over
// this protocol. The name is still sent, because it is what a KeePassXC that
// ever starts honouring it would use.
const keepassxcGroup = "SSHakku"

// ErrDeleteUnsupported is returned by a backend that can store and read a
// passphrase but cannot remove one. KeePassXC's local protocol has verbs for
// reading and writing an entry and none for deleting it, so a passphrase kept
// this way can only be removed by the user, in KeePassXC.
//
// It is reported rather than worked around. Blanking the entry, or moving it
// out of the way so the next lookup misses, would let SSHakku say a passphrase
// was forgotten while it is still sitting in the database — a false claim about
// a secret, which is worse than an honest refusal.
var ErrDeleteUnsupported = errors.New("secret backend cannot delete a stored entry")

// AssociationStore persists the association KeePassXC hands out, so the user
// approves this client once rather than on every run.
type AssociationStore interface {
	Load() (keepassxc.Association, bool, error)
	Save(keepassxc.Association) error
}

// KeePassXCSession is the part of KeePassXC's protocol this backend uses. It
// is an interface so the backend's own decisions — which entry matches, when an
// approval may be asked for — can be driven without a running KeePassXC. The
// protocol underneath is verified separately, against a server that speaks it
// for real.
type KeePassXCSession interface {
	TestAssociate(keepassxc.Association) error
	Associate() (keepassxc.Association, error)
	GetLogins(url string, a keepassxc.Association) ([]keepassxc.Entry, error)
	SetLogin(url, login, password, uuid, group string, a keepassxc.Association) error
	Close() error
}

// KeePassXCBackend keeps passphrases in a running, unlocked KeePassXC, reached
// over its local socket protocol. The passphrase travels encrypted over that
// socket and never through argv or the environment.
//
// It needs KeePassXC to be running with its database unlocked and browser
// integration enabled. That is the same precondition as reaching KeePassXC
// through the Secret Service, and it is what lets a lookup stay silent.
type KeePassXCBackend struct {
	// NewSession opens a session with KeePassXC. A nil NewSession dials the
	// candidate socket paths in turn and speaks the real protocol.
	NewSession func() (KeePassXCSession, error)
	// SocketPaths are the endpoints to try, in order. Empty means the
	// platform's usual places.
	SocketPaths []string
	// Associations persists the approval this client was granted.
	Associations AssociationStore
	// Group is the group name sent with a new entry; empty selects the one
	// SSHakku names by default. KeePassXC decides the placement itself (see
	// keepassxcGroup), so this changes what is sent and not where the entry
	// lands.
	Group string
	// Timeout bounds each exchange KeePassXC answers on its own; zero selects
	// the protocol's default.
	Timeout time.Duration
	// InteractiveTimeout bounds the one exchange that waits on a person: the
	// approval dialog KeePassXC raises the first time this client associates.
	// Zero selects the protocol's default.
	InteractiveTimeout time.Duration
}

// group is the group name to send: the configured one, else SSHakku's own.
func (b KeePassXCBackend) group() string {
	if b.Group != "" {
		return b.Group
	}
	return keepassxcGroup
}

// entryURL is the URL a key's entry is stored under.
func entryURL(service string) string {
	return keepassxcURLScheme + "://" + service
}

// connect opens a session with KeePassXC.
func (b KeePassXCBackend) connect() (KeePassXCSession, error) {
	if b.NewSession != nil {
		return b.NewSession()
	}
	conn, err := dialKeePassXCAt(b.socketPaths())
	if err != nil {
		return nil, err
	}
	client, err := keepassxc.Connect(conn, b.Timeout, b.InteractiveTimeout)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return client, nil
}

// socketPaths is where to look for KeePassXC: what the caller configured, else
// the platform's usual places.
func (b KeePassXCBackend) socketPaths() []string {
	if len(b.SocketPaths) > 0 {
		return b.SocketPaths
	}
	return keepassxc.SocketPaths()
}

// dialKeePassXCAt tries each candidate socket in turn and reports every path it
// tried when none answers, so a user can see where it looked.
func dialKeePassXCAt(paths []string) (net.Conn, error) {
	for _, path := range paths {
		conn, err := net.Dial("unix", path)
		if err == nil {
			return conn, nil
		}
	}
	return nil, fmt.Errorf("%w (tried %s)", keepassxc.ErrNotRunning, strings.Join(paths, ", "))
}

// association returns the stored association, having confirmed KeePassXC still
// honours it. It never asks for a new one: associating raises a dialog, and a
// lookup that pops a dialog is exactly the silence a stored passphrase exists
// to preserve.
func (b KeePassXCBackend) association(client KeePassXCSession) (keepassxc.Association, error) {
	if b.Associations == nil {
		return keepassxc.Association{}, errors.New("keepassxc: no association store was configured")
	}
	stored, found, err := b.Associations.Load()
	if err != nil {
		return keepassxc.Association{}, err
	}
	if !found {
		return keepassxc.Association{}, keepassxc.ErrNotAssociated
	}
	if err := client.TestAssociate(stored); err != nil {
		return keepassxc.Association{}, err
	}
	return stored, nil
}

// Lookup returns the passphrase stored for service.
//
// KeePassXC matches by URL, and its URL host is case-insensitive, so two key
// names differing only in case resolve to the same URL. The exact name is also
// written to the entry's username, and that is what decides here — the URL
// narrows the candidates, the exact comparison picks among them.
func (b KeePassXCBackend) Lookup(ctx context.Context, service string) (string, bool, error) {
	client, err := b.connect()
	if err != nil {
		return "", false, err
	}
	defer func() { _ = client.Close() }()

	assoc, err := b.association(client)
	if err != nil {
		return "", false, err
	}
	entries, err := client.GetLogins(entryURL(service), assoc)
	if err != nil {
		return "", false, err
	}
	for _, entry := range entries {
		if entry.Login == service {
			return entry.Password, true, nil
		}
	}
	return "", false, nil
}

// Store saves passphrase for service, replacing the entry in place when one is
// already there so a re-stored passphrase does not leave a second copy behind.
//
// This is where a first-time association is asked for. It raises a dialog the
// user has to approve, and this is the moment to do it: the user is already
// present — they have just been asked for the passphrase — whereas a lookup
// happens with nobody watching.
func (b KeePassXCBackend) Store(ctx context.Context, service, label, passphrase string) error {
	client, err := b.connect()
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	assoc, err := b.association(client)
	if errors.Is(err, keepassxc.ErrNotAssociated) {
		assoc, err = b.associate(client)
	}
	if err != nil {
		return err
	}

	url := entryURL(service)
	uuid := ""
	if entries, lookupErr := client.GetLogins(url, assoc); lookupErr == nil {
		for _, entry := range entries {
			if entry.Login == service {
				uuid = entry.UUID
				break
			}
		}
	}
	// label has nowhere to go: set-login carries a URL, a username and a
	// password, and KeePassXC titles the entry from the URL itself. The key
	// name travels as the username instead, which is also what Lookup matches
	// on.
	_ = label
	return client.SetLogin(url, service, passphrase, uuid, b.group(), assoc)
}

// associate registers this client and remembers the result.
func (b KeePassXCBackend) associate(client KeePassXCSession) (keepassxc.Association, error) {
	assoc, err := client.Associate()
	if err != nil {
		return keepassxc.Association{}, err
	}
	if err := b.Associations.Save(assoc); err != nil {
		return keepassxc.Association{}, err
	}
	return assoc, nil
}

// Delete reports that this route cannot remove an entry. See
// ErrDeleteUnsupported: the protocol has no verb for it, and pretending
// otherwise would mean telling the user a passphrase is gone while it is not.
func (b KeePassXCBackend) Delete(ctx context.Context, service string) error {
	return fmt.Errorf("%w: remove the %q entry in KeePassXC to forget it", ErrDeleteUnsupported, entryURL(service))
}

// List cannot enumerate SSHakku's entries: the protocol looks entries up by
// URL, one URL at a time, and offers no way to ask which ones exist.
func (b KeePassXCBackend) List(ctx context.Context) ([]string, error) {
	return nil, ErrListUnsupported
}
