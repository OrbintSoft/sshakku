package keys

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/OrbintSoft/sshakku/internal/keys/prompt"
	"github.com/OrbintSoft/sshakku/internal/keys/wallet"
	"github.com/OrbintSoft/sshakku/internal/logline"
	"github.com/OrbintSoft/sshakku/internal/run"
)

const defaultMaxAttempts = 3

// DefaultMaxAttempts is how many passphrase attempts a key gets when the
// configuration asks for no particular number. The loader applies it; anything
// that has to state what is in force reads it here rather than repeating the 3.
const DefaultMaxAttempts = defaultMaxAttempts

// Logger records one level-tagged line. A nil Logger disables logging.
type Logger interface {
	Log(level, message string) error
}

// KeyLister lists the private-key files to consider, in load order.
type KeyLister interface {
	Keys() ([]string, error)
}

// KeyAdder adds one private key to the agent via ssh-add, returning ssh-add's
// exit code (0 = added). A non-zero code is a normal "wrong passphrase" outcome,
// reported in the int; only a failure to run ssh-add is an error.
type KeyAdder interface {
	// AddWithAskpass adds keyfile, handing passphrase to ssh-add out of band
	// through the keyring + SSH_ASKPASS helper, so it never appears in argv.
	AddWithAskpass(ctx context.Context, keyfile, passphrase string) (int, error)
}

// GiveupStore persists, per key, that loading was abandoned after the bounded
// retries, so later shells skip the key instead of re-prompting on every new
// terminal. A nil GiveupStore disables give-up.
type GiveupStore interface {
	// GivenUp reports whether the key is currently in the give-up state.
	GivenUp(key string) bool
	// Record marks the key as given up after its retries were exhausted.
	Record(key string) error
	// Clear removes any give-up record for the key (e.g. after a success).
	Clear(key string) error
}

// KeyState records, per key, when it was added to the agent and for how long
// — so `sshakku doctor` can later report its remaining time there, which the
// ssh-agent protocol itself has no query for, and so a session can find a key
// whose time is up. A nil KeyState disables recording.
type KeyState interface {
	// Save records that the key in keyfile was just added to the agent with
	// lifetime (zero meaning no expiry).
	Save(keyfile string, lifetime time.Duration) error
}

// Notifier surfaces a user-facing notice — to the terminal of the interactive
// shell that ran the loader — about a problem the user should act on, such as a
// key that could not be loaded. A nil Notifier suppresses notices; the success
// path never notifies.
type Notifier interface {
	Notify(message string)
}

// Config tunes a Loader.
type Config struct {
	// ServicePrefix prefixes the per-key secret-store service; "" uses the
	// default (see defaultServicePrefix).
	ServicePrefix string
	// MaxAttempts bounds the retries per key; <1 uses 3.
	MaxAttempts int
	// WalletStore reports whether keyname's passphrase should be persisted to
	// the secret store; nil stores every key (today's behaviour). An excluded
	// key is still used normally in-session — only the persistent store is
	// skipped.
	WalletStore func(keyname string) bool
	// AutoLoad reports whether keyname should be proactively added to the
	// agent at shell-init; nil loads every key (today's behaviour). An
	// excluded key is simply never considered here — it can still be added on
	// demand via the askpass broker, which does not consult this policy.
	AutoLoad func(keyname string) bool
	// KeyLifetime is how long a key added here is meant to stay in the agent,
	// which is what the user configured — not necessarily what the agent was
	// told. Where the agent holds lifetimes it is handed the same value and
	// enforces it (ExecKeyAdder.KeyLifetime); where it holds none it is handed
	// nothing, and the value still means what it says, because the record
	// written from it is what a later session goes by when it takes an expired
	// key back out. The Loader itself only records it.
	KeyLifetime time.Duration
	// OnDismiss is what closing a passphrase prompt without answering means
	// for the keys that come after it: one of the OnDismiss* values, with ""
	// meaning OnDismissStop.
	OnDismiss string
}

// What closing a passphrase prompt without answering means, for Config.OnDismiss.
// Whichever applies, the dismissal itself stores nothing and gives up on no key:
// the next login shell asks again from the first key.
const (
	// OnDismissStop asks about no further key for the rest of the session, so
	// shutting one window does not leave the user with one more per key.
	OnDismissStop = "stop"
	// OnDismissSkip turns down that key alone and goes on asking about the rest.
	OnDismissSkip = "skip"
	// OnDismissRetry treats the dismissal as a wrong answer: the same key is
	// asked about again until the attempts run out.
	OnDismissRetry = "retry"
)

// Loader loads the user's keys into the agent, skipping any already present and
// pulling passphrases from the secret store (or prompting) when needed.
type Loader struct {
	Keys     KeyLister
	Runner   run.Runner
	Secret   wallet.Backend
	Prompt   prompt.Prompter
	Adder    KeyAdder
	Log      Logger
	Notify   Notifier
	Giveup   GiveupStore
	KeyState KeyState
	Config   Config
}

// LoadKeys enumerates the keys, snapshots the agent's loaded fingerprints once,
// and loads each missing key (best-effort: a failure on one key is logged and the
// rest still run). It returns an error only when the keys cannot be enumerated or
// the agent cannot be queried.
//
// When the secret backend supports it (wallet.Session), the whole batch shares a
// single wallet unlock instead of one per key: the wallet opens lazily on the
// first key that actually needs it and closes once the batch is done, rather
// than once per key or waiting out the wallet's own idle timeout.
func (l Loader) LoadKeys(ctx context.Context) error {
	keyfiles, err := l.Keys.Keys()
	if err != nil {
		return fmt.Errorf("enumerate keys: %w", err)
	}
	if len(keyfiles) == 0 {
		l.logf("INFO", "no keys to load")
		return nil
	}
	loaded, err := AgentFingerprints(ctx, l.Runner)
	if err != nil {
		return fmt.Errorf("read agent fingerprints: %w", err)
	}

	if sess, ok := l.Secret.(wallet.Session); ok {
		sb := &sessionBackend{Backend: l.Secret, sess: sess}
		l.Secret = sb
		defer func() {
			if !sb.unlocked {
				return
			}
			if err := sess.Lock(ctx); err != nil {
				l.logf("ERROR", "lock secret store: %v", err)
			}
		}()
	}

	for _, keyfile := range keyfiles {
		if l.loadOne(ctx, keyfile, loaded) {
			break
		}
	}
	return nil
}

// sessionBackend wraps a wallet.Backend so the first Lookup or Store made
// through it unlocks the underlying wallet.Session, and every call after that
// reuses the same unlock instead of triggering its own. The holder (LoadKeys)
// locks it back up once, after the whole batch — not after each call.
type sessionBackend struct {
	wallet.Backend
	sess     wallet.Session
	unlocked bool
}

// ensureUnlocked unlocks the session on first use; a failed unlock is left for
// the wrapped backend's own Lookup/Store to report, so it still falls back to
// prompting rather than failing the whole batch.
func (s *sessionBackend) ensureUnlocked(ctx context.Context) {
	if s.unlocked {
		return
	}
	if err := s.sess.Unlock(ctx); err == nil {
		s.unlocked = true
	}
}

func (s *sessionBackend) Lookup(ctx context.Context, service string) (string, bool, error) {
	s.ensureUnlocked(ctx)
	return s.Backend.Lookup(ctx, service)
}

func (s *sessionBackend) Store(ctx context.Context, service, label, passphrase string) error {
	s.ensureUnlocked(ctx)
	return s.Backend.Store(ctx, service, label, passphrase)
}

// loadOne loads a single key unless its fingerprint is already in the agent. It
// reports whether the user's answer ends the asking for the keys still to come.
func (l Loader) loadOne(ctx context.Context, keyfile string, loaded map[string]bool) (askingEnded bool) {
	keyname := filepath.Base(keyfile)

	if !autoLoads(l.Config, keyname) {
		l.logf("INFO", "auto-load policy excludes %s, skipping", keyname)
		return false
	}

	fp, err := FileFingerprint(ctx, l.Runner, keyfile)
	if err != nil {
		// ssh-keygen could not run; dedup is impossible, but ssh-add may still
		// add the key, so press on rather than skip it.
		l.logf("ERROR", "fingerprint %s: %v", keyname, err)
	}
	if fp != "" && loaded[fp] {
		l.logf("INFO", "%s already added to agent", keyname)
		return false
	}
	if l.givenUp(keyname) {
		l.logf("INFO", "%s given up earlier, skipping until the retry window", keyname)
		return false
	}
	return l.addWithRetries(ctx, keyfile, keyname)
}

// keyOutcome is what came of trying to load one key.
type keyOutcome int

const (
	// keyAbandoned: nothing more to try for this key, and nothing to record
	// against it — a dismissed prompt, no terminal to ask on, or a hard error.
	keyAbandoned keyOutcome = iota
	keyLoaded
	// attemptsExhausted: the user was asked as often as they allowed and the
	// key never opened.
	attemptsExhausted
	// askingEnded: the user dismissed the prompt, and wants no more of them.
	askingEnded
)

// addWithRetries loads keyfile, retrying on a wrong passphrase up to MaxAttempts
// times. On success it clears any give-up record; when the attempts are
// exhausted it gives up persistently and notifies the user. It reports whether
// the asking is over for the rest of the session.
func (l Loader) addWithRetries(ctx context.Context, keyfile, keyname string) bool {
	max := l.Config.MaxAttempts
	if max < 1 {
		max = defaultMaxAttempts
	}

	switch l.loadViaVaultThenPrompt(ctx, keyfile, keyname, max) {
	case keyLoaded:
		l.clearGiveup(keyname)
		l.saveKeyState(keyfile)
	case attemptsExhausted:
		l.logf("ERROR", "giving up on %s after %d attempts", keyname, max)
		l.notify("could not load key %s after %d attempts", keyname, max)
		l.recordGiveup(keyname)
	case askingEnded:
		return true
	}
	return false
}

// loadViaVaultThenPrompt tries a stored passphrase once (a silent success on the
// happy path), then prompts the user up to max times, storing the first prompted
// passphrase that works. A stored passphrase that ssh-add rejects is treated as
// stale and dropped in favour of prompting.
func (l Loader) loadViaVaultThenPrompt(ctx context.Context, keyfile, keyname string, max int) keyOutcome {
	service := l.servicePrefix() + "-" + keyname

	if pass, ok := l.storedPassphrase(ctx, service, keyname); ok {
		rc, err := l.Adder.AddWithAskpass(ctx, keyfile, pass)
		if err != nil {
			l.failAdd(keyname, err)
			return keyAbandoned
		}
		if rc == 0 {
			l.logf("INFO", "added %s to agent", keyname)
			return keyLoaded
		}
		l.logf("INFO", "stored passphrase for %s is stale, prompting", keyname)
	}

	for attempt := 1; attempt <= max; attempt++ {
		pass, err := l.Prompt.Prompt(ctx, keyname)
		if err != nil {
			switch {
			case errors.Is(err, prompt.ErrCanceled):
				// Turning the question down is an answer, not a fault: it is
				// logged the way the other expected outcomes are, and what it
				// means for the keys still to come is the user's to configure.
				switch l.Config.OnDismiss {
				case OnDismissRetry:
					l.logf("INFO", "passphrase prompt dismissed for %s (attempt %d/%d)", keyname, attempt, max)
					continue
				case OnDismissSkip:
					l.logf("INFO", "passphrase prompt dismissed for %s", keyname)
					return keyAbandoned
				default:
					l.logf("INFO", "passphrase prompt dismissed for %s, asking about no further key this session", keyname)
					return askingEnded
				}
			case errors.Is(err, prompt.ErrNoTerminal):
				// No GUI and no controlling terminal are both normal, expected
				// deployments — not surfaced to the user, and not logged as an
				// operator problem.
				l.logf("INFO", "no terminal available to prompt for %s", keyname)
			default:
				l.failPrompt(keyname, err)
			}
			return keyAbandoned
		}
		if pass == "" {
			// An empty answer opens no key — a key that has no passphrase is
			// never asked about — so it costs an attempt here rather than being
			// handed on to ssh-add.
			l.logf("ERROR", "empty passphrase for %s (attempt %d/%d)", keyname, attempt, max)
			continue
		}
		rc, err := l.Adder.AddWithAskpass(ctx, keyfile, pass)
		if err != nil {
			l.failAdd(keyname, err)
			return keyAbandoned
		}
		if rc == 0 {
			l.logf("INFO", "added %s to agent", keyname)
			l.storePassphrase(ctx, service, keyname, pass)
			return keyLoaded
		}
		l.logf("ERROR", "failed to add %s (attempt %d/%d)", keyname, attempt, max)
	}
	return attemptsExhausted
}

// storedPassphrase returns the stored passphrase for service and whether a
// non-empty one was found. A lookup error is logged at INFO, not ERROR: it is
// usually just the configured backend not being reachable in this
// environment (no D-Bus session, no GUI) — an expected, recoverable miss, not
// an operator problem — and is treated the same way as "no entry found".
func (l Loader) storedPassphrase(ctx context.Context, service, keyname string) (string, bool) {
	pass, found, err := l.Secret.Lookup(ctx, service)
	if err != nil {
		l.logf("INFO", "secret lookup for %s: %v", keyname, err)
		return "", false
	}
	if found && strings.TrimSpace(pass) != "" {
		l.logf("INFO", "using stored passphrase for %s", keyname)
		return pass, true
	}
	l.logf("INFO", "no stored passphrase for %s, prompting", keyname)
	return "", false
}

// failAdd logs and notifies a failure to run ssh-add for a key.
func (l Loader) failAdd(keyname string, err error) {
	l.logf("ERROR", "add %s: %v", keyname, err)
	l.notify("could not load key %s: %v", keyname, err)
}

// failPrompt logs and notifies a non-cancel failure to obtain a passphrase.
func (l Loader) failPrompt(keyname string, err error) {
	l.logf("ERROR", "prompt %s: %v", keyname, err)
	l.notify("could not load key %s: %v", keyname, err)
}

// storePassphrase saves a freshly prompted passphrase after a successful add,
// unless the wallet-store policy excludes keyname. Storing is best-effort: the
// key is already in the agent if this fails.
func (l Loader) storePassphrase(ctx context.Context, service, keyname, passphrase string) {
	if !walletStores(l.Config, keyname) {
		l.logf("INFO", "wallet-store policy excludes %s, not storing", keyname)
		return
	}
	if err := storeInWallet(ctx, l.Secret, service, keyname, passphrase); err != nil {
		l.logf("ERROR", "store passphrase for %s: %v", keyname, err)
		return
	}
	l.logf("INFO", "stored passphrase for %s", keyname)
}

// givenUp reports whether give-up tracking is enabled and the key is currently
// in the give-up state.
func (l Loader) givenUp(keyname string) bool {
	return l.Giveup != nil && l.Giveup.GivenUp(keyname)
}

// recordGiveup persists that the key was abandoned after its retries, best-effort.
func (l Loader) recordGiveup(keyname string) {
	if l.Giveup == nil {
		return
	}
	if err := l.Giveup.Record(keyname); err != nil {
		l.logf("ERROR", "record give-up for %s: %v", keyname, err)
	}
}

// clearGiveup drops any give-up record after a successful add, best-effort.
func (l Loader) clearGiveup(keyname string) {
	if l.Giveup == nil {
		return
	}
	if err := l.Giveup.Clear(keyname); err != nil {
		l.logf("ERROR", "clear give-up for %s: %v", keyname, err)
	}
}

// saveKeyState records that the key in keyfile was just added to the agent
// with the configured lifetime, best-effort: a failure to persist this
// bookkeeping must not fail an otherwise-successful key load.
func (l Loader) saveKeyState(keyfile string) {
	if l.KeyState == nil {
		return
	}
	if err := l.KeyState.Save(keyfile, l.Config.KeyLifetime); err != nil {
		l.logf("ERROR", "record key state for %s: %v", filepath.Base(keyfile), err)
	}
}

func (l Loader) servicePrefix() string {
	return servicePrefixOf(l.Config)
}

// autoLoads reports whether keyname should be proactively added to the agent
// under c's auto-load policy; a nil AutoLoad loads every key.
func autoLoads(c Config, keyname string) bool {
	if c.AutoLoad == nil {
		return true
	}
	return c.AutoLoad(keyname)
}

func (l Loader) logf(level, format string, args ...any) {
	logline.Recordf(l.Log, level, format, args...)
}

// notify emits a user-facing notice when a Notifier is configured.
func (l Loader) notify(format string, args ...any) {
	if l.Notify == nil {
		return
	}
	l.Notify.Notify(fmt.Sprintf(format, args...))
}

var _ KeyLister = Enumerator{}
