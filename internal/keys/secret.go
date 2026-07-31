package keys

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// jsonMarshal is the item-template encoder the shell-out backends (Bitwarden,
// 1Password) use to build their stdin payloads. It is a seam so the otherwise
// unreachable marshal-failure branch of each Store can be exercised.
var jsonMarshal = json.Marshal

// ErrListUnsupported is returned by List on a backend that cannot enumerate
// its stored entries — e.g. SecretToolBackend, which has no generic
// "list everything" verb without already knowing exact attributes.
var ErrListUnsupported = errors.New("secret backend does not support listing stored entries")

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
	// Timeout bounds each secret-tool call. The wallet may be locked behind an
	// unlock prompt that nobody can answer, and something is waiting on the
	// answer — a login shell, or an ssh at a passphrase prompt — so the wait is
	// finite and the caller falls back to asking on the terminal. Zero selects
	// DefaultCommandTimeout.
	Timeout time.Duration
}

// run bounds every secret-tool call.
func (b SecretToolBackend) run(c Cmd) (Result, error) {
	if c.Timeout <= 0 {
		c.Timeout = b.Timeout
	}
	return b.Runner.Run(c)
}

// Lookup runs `secret-tool lookup service <service> username <user>`. secret-tool
// emits the secret verbatim, so a trailing newline (e.g. from an entry stored by
// the earlier shell version) is trimmed. A non-zero exit means no entry — handled
// as a miss, not an error, so the loader falls back to prompting.
func (b SecretToolBackend) Lookup(service string) (string, bool, error) {
	res, err := b.run(Cmd{
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
	res, err := b.run(Cmd{
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
	res, err := b.run(Cmd{
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
