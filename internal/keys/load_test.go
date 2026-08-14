package keys

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/keys/prompt"
	"github.com/OrbintSoft/sshakku/internal/keys/wallet"
	"github.com/OrbintSoft/sshakku/internal/run"
	"github.com/OrbintSoft/sshakku/internal/run/runtest"
)

// assertNothingWentWrong reads the session log the way a user would when they
// go looking for a problem: an ERROR line is the product saying something is
// broken, and several outcomes below are ordinary rather than broken.
func assertNothingWentWrong(t *testing.T, log *fakeLogger, why string) {
	t.Helper()
	for _, line := range log.lines {
		assert.Falsef(t, strings.HasPrefix(line, "ERROR"), "%s, got %v", why, log.lines)
	}
}

// agentEmpty answers `ssh-add -l` as an empty agent; keygen answers `ssh-keygen
// -lf` with a fingerprint line for a key file.
func agentEmpty() func(run.Cmd) (run.Result, error) {
	return runtest.Stdout("The agent has no identities.\n", 1)
}

func keygen(fp string) func(run.Cmd) (run.Result, error) {
	return runtest.Stdout("256 "+fp+" comment (ED25519)\n", 0)
}

func TestLoadKeysSkipsLoaded(t *testing.T) {
	r := runtest.NewRunner().
		On("ssh-add", runtest.Stdout("256 SHA256:DUP loaded (ED25519)\n", 0)).
		On("ssh-keygen", keygen("SHA256:DUP"))
	adder := &fakeKeyAdder{}
	log := &fakeLogger{}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_x"}},
		Runner: r,
		Adder:  adder,
		Log:    log,
		Config: Config{},
	}
	require.NoError(t, l.LoadKeys(t.Context()), "loading a login's keys must not fail")
	assert.Emptyf(t, adder.calls, "a key the agent already holds must not be added again: %+v", adder.calls)
	assert.Truef(t, log.contains("already added"), "and the log must say why nothing happened: %v", log.lines)
}

func TestLoadKeysStoredPassphrase(t *testing.T) {
	r := runtest.NewRunner().On("ssh-add", agentEmpty()).On("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupPass: "stored-pass", lookupFound: true}
	adder := &fakeKeyAdder{withCodes: []int{0}}
	log := &fakeLogger{}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: secret, Prompt: &fakePrompter{}, Adder: adder, Log: log,
		Config: Config{},
	}
	require.NoError(t, l.LoadKeys(t.Context()), "loading a login's keys must not fail")
	require.Lenf(t, adder.calls, 1, "one key that is not loaded is one add: %+v", adder.calls)
	assert.Equal(t, "stored-pass", adder.calls[0].passphrase,
		"the passphrase in the wallet is what opens the key, without asking anyone")
	assert.Emptyf(t, secret.stored, "and what came out of the wallet must not be written back into it: %v", secret.stored)
}

func TestLoadKeysPromptThenStore(t *testing.T) {
	r := runtest.NewRunner().On("ssh-add", agentEmpty()).On("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupFound: false}
	prompter := &fakePrompter{pass: "typed-pass"}
	adder := &fakeKeyAdder{withCodes: []int{0}}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: secret, Prompt: prompter, Adder: adder, Log: &fakeLogger{},
		Config: Config{},
	}
	require.NoError(t, l.LoadKeys(t.Context()), "loading a login's keys must not fail")
	require.Lenf(t, adder.calls, 1, "one key that is not loaded is one add: %+v", adder.calls)
	assert.Equal(t, "typed-pass", adder.calls[0].passphrase, "opened with the passphrase the user typed")
	require.Lenf(t, secret.stored, 1, "and a passphrase typed once must be saved once: %v", secret.stored)
	got := secret.stored[0]
	assert.Equal(t, wallet.DefaultServicePrefix+"-id_rsa", got.service, "under the name a later lookup goes by")
	assert.Equal(t, "SSH Passphrase for id_rsa", got.label, "labelled with what a person sees in their wallet")
	assert.Equal(t, "typed-pass", got.passphrase, "and holding what they typed, so the next login is silent")
}

// TestLoadKeysLookupErrorLogsInfoNotError confirms a lookup error (the
// backend being unreachable in this environment, e.g. no D-Bus session) is
// logged at INFO and still falls through to prompting — not treated as an
// operator-actionable ERROR the way a genuine failure later (a rejected
// stored passphrase, an exhausted retry loop) still is.
func TestLoadKeysLookupErrorLogsInfoNotError(t *testing.T) {
	r := runtest.NewRunner().On("ssh-add", agentEmpty()).On("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupErr: errors.New("dbus: not reachable")}
	prompter := &fakePrompter{pass: "typed-pass"}
	adder := &fakeKeyAdder{withCodes: []int{0}}
	log := &fakeLogger{}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: secret, Prompt: prompter, Adder: adder, Log: log,
	}
	require.NoError(t, l.LoadKeys(t.Context()), "loading a login's keys must not fail")
	require.Lenf(t, adder.calls, 1, "a wallet nobody could reach must not cost the user their key: %+v", adder.calls)
	assert.Equal(t, "typed-pass", adder.calls[0].passphrase, "they are asked instead, and the key opens")
	assert.Truef(t, log.contains("INFO secret lookup"), "the log must say the wallet was not reached: %v", log.lines)
	assertNothingWentWrong(t, log,
		"a machine with no wallet in this session is not a machine with something wrong with it")
}

func TestLoadKeysPromptThenStoreExcludedByPolicy(t *testing.T) {
	r := runtest.NewRunner().On("ssh-add", agentEmpty()).On("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupFound: false}
	prompter := &fakePrompter{pass: "typed-pass"}
	adder := &fakeKeyAdder{withCodes: []int{0}}
	log := &fakeLogger{}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: secret, Prompt: prompter, Adder: adder, Log: log,
		Config: Config{WalletStore: func(keyname string) bool { return keyname != "id_rsa" }},
	}
	require.NoError(t, l.LoadKeys(t.Context()), "loading a login's keys must not fail")
	require.Lenf(t, adder.calls, 1, "the key must still be loaded: %+v", adder.calls)
	assert.Equal(t, "typed-pass", adder.calls[0].passphrase, "with the passphrase the user typed")
	assert.Emptyf(t, secret.stored,
		"but a key the user excluded from their wallet must not end up in it: %v", secret.stored)
	assert.Truef(t, log.contains("wallet-store policy excludes id_rsa"),
		"and the log must say why it was not saved, or the setting looks ignored: %v", log.lines)
}

func TestLoadKeysAutoLoadExcludedByPolicyNeverAdded(t *testing.T) {
	// ssh-keygen is deliberately not registered: an excluded key must never
	// reach the fingerprint lookup, only the agent snapshot (ssh-add -l).
	r := runtest.NewRunner().On("ssh-add", agentEmpty())
	adder := &fakeKeyAdder{}
	log := &fakeLogger{}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Adder: adder, Log: log,
		Config: Config{AutoLoad: func(keyname string) bool { return keyname != "id_rsa" }},
	}
	require.NoError(t, l.LoadKeys(t.Context()), "loading a login's keys must not fail")
	assert.Emptyf(t, adder.calls, "a key the user excluded from loading must not be loaded: %+v", adder.calls)
	require.Lenf(t, r.Calls, 1, "nor touched beyond the one snapshot of what the agent holds: %v", r.Calls)
	assert.Equal(t, "ssh-add", r.Calls[0].Name, "and that snapshot is the only command that may run")
	assert.Truef(t, log.contains("auto-load policy excludes id_rsa"),
		"the log must say why the key was left alone, or the setting looks ignored: %v", log.lines)
}

func TestLoadKeysRetriesThenGivesUp(t *testing.T) {
	r := runtest.NewRunner().On("ssh-add", agentEmpty()).On("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupPass: "stale", lookupFound: true}
	prompter := &fakePrompter{pass: "still-wrong"}
	// The stale stored passphrase gets one try, then three prompted attempts; all fail.
	adder := &fakeKeyAdder{withCodes: []int{1, 1, 1, 1}}
	log := &fakeLogger{}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: secret, Prompt: prompter, Adder: adder, Log: log,
		Config: Config{},
	}
	require.NoError(t, l.LoadKeys(t.Context()), "loading a login's keys must not fail")
	assert.Lenf(t, adder.calls, 4,
		"the stored passphrase is tried once, then the user is asked three times: %+v", adder.calls)
	assert.Truef(t, log.contains("attempt 3/3"), "the log must number the attempts: %v", log.lines)
	assert.Truef(t, log.contains("giving up"), "and say the asking stopped: %v", log.lines)
}

// TestLoadKeysEmptyAnswerIsAWrongAnswer verifies F8 for the answer people give
// by accident: Enter on its own. An empty passphrase opens no key — a key
// without one is never asked about — so it is a wrong passphrase like any
// other, counted towards the bounded retries and asked again, and the user is
// told once at the end rather than losing the key on the first press.
//
// It never reaches the key adder either. Where the handoff is the kernel
// keyring, an empty payload is refused outright (see the keyring package), and
// an error there abandons the key; where it is a socket it is accepted and only
// ssh-add rejects it, so the same keystroke behaves differently on the two
// systems for no reason a user could see.
func TestLoadKeysEmptyAnswerIsAWrongAnswer(t *testing.T) {
	r := runtest.NewRunner().On("ssh-add", agentEmpty()).On("ssh-keygen", keygen("SHA256:NEW"))
	prompter := &fakePrompter{pass: ""}
	adder := &fakeKeyAdder{}
	log := &fakeLogger{}
	notes := &fakeNotifier{}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: &fakeSecret{}, Prompt: prompter, Adder: adder, Log: log,
		Notify: notes, Config: Config{MaxAttempts: 3},
	}
	require.NoError(t, l.LoadKeys(t.Context()), "loading a login's keys must not fail")
	assert.Len(t, prompter.calls, 3,
		"Enter on its own opens no key, so it is a wrong answer like any other and the user is asked again")
	for _, c := range adder.calls {
		assert.NotEmptyf(t, c.passphrase,
			"and it must never be handed on: a kernel keyring refuses an empty payload outright while a socket "+
				"takes it, so the same keystroke would behave differently on the two systems: %+v", adder.calls)
	}
	assert.Truef(t, log.contains("giving up"), "the log must say the asking stopped: %v", log.lines)
	assert.Lenf(t, notes.msgs, 1, "and the user is told once at the end, not on every press: %v", notes.msgs)
}

// TestLoadKeysBlankStoredPassphraseIsNoPassphrase is the wallet's half of what
// TestLoadKeysEmptyAnswerIsAWrongAnswer says about the keyboard: an entry
// holding nothing but blank space is not a passphrase, and handing it on has
// the same consequence — where the handoff is the kernel keyring an empty
// payload is refused outright, and an error there abandons the key instead of
// asking the user for it.
//
// A wallet entry can end up blank; the user is not the one who would find out.
func TestLoadKeysBlankStoredPassphraseIsNoPassphrase(t *testing.T) {
	r := runtest.NewRunner().On("ssh-add", agentEmpty()).On("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupPass: "   ", lookupFound: true}
	prompter := &fakePrompter{pass: "typed-pass"}
	adder := &fakeKeyAdder{withCodes: []int{0}}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: secret, Prompt: prompter, Adder: adder, Log: &fakeLogger{},
		Config: Config{},
	}
	require.NoError(t, l.LoadKeys(t.Context()), "loading a login's keys must not fail")

	for _, c := range adder.calls {
		assert.NotEmptyf(t, strings.TrimSpace(c.passphrase),
			"a wallet entry holding only blank space must never be handed on as the passphrase: %+v", adder.calls)
	}
	require.Lenf(t, adder.calls, 1, "the user is asked instead, and the key opens: %+v", adder.calls)
	assert.Equal(t, "typed-pass", adder.calls[0].passphrase, "with what they typed")
	require.Lenf(t, secret.stored, 1, "and the blank entry must be corrected: %v", secret.stored)
	assert.Equal(t, "typed-pass", secret.stored[0].passphrase,
		"or every later login repeats this whole exchange")
}

func TestLoadKeysStaleStoredThenPromptStores(t *testing.T) {
	r := runtest.NewRunner().On("ssh-add", agentEmpty()).On("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupPass: "stale", lookupFound: true}
	prompter := &fakePrompter{pass: "fresh"}
	adder := &fakeKeyAdder{withCodes: []int{1, 0}} // stored rejected, prompted accepted
	log := &fakeLogger{}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: secret, Prompt: prompter, Adder: adder, Log: log,
		Config: Config{},
	}
	require.NoError(t, l.LoadKeys(t.Context()), "loading a login's keys must not fail")
	require.Lenf(t, adder.calls, 2, "the stored passphrase is tried, then the user is asked: %+v", adder.calls)
	assert.Equal(t, "stale", adder.calls[0].passphrase, "the one in the wallet goes first")
	assert.Equal(t, "fresh", adder.calls[1].passphrase, "and the one they typed second")
	require.Lenf(t, secret.stored, 1, "the wallet must be corrected, once: %v", secret.stored)
	assert.Equal(t, "fresh", secret.stored[0].passphrase,
		"holding what works, or every later login repeats this whole exchange")
	assert.Truef(t, log.contains("is stale"), "and the log must say the saved one no longer opened the key: %v", log.lines)
}

func TestLoadKeysNotifiesOnGiveup(t *testing.T) {
	r := runtest.NewRunner().On("ssh-add", agentEmpty()).On("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupFound: false}
	prompter := &fakePrompter{pass: "wrong"}
	adder := &fakeKeyAdder{withCodes: []int{1, 1, 1}}
	notifier := &fakeNotifier{}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: secret, Prompt: prompter, Adder: adder, Log: &fakeLogger{},
		Notify: notifier, Config: Config{},
	}
	require.NoError(t, l.LoadKeys(t.Context()), "loading a login's keys must not fail")
	require.Lenf(t, notifier.msgs, 1, "a key that never opened is worth telling the user about, once: %v", notifier.msgs)
	assert.Contains(t, notifier.msgs[0], "could not load key id_rsa", "and the notice must name the key")
}

func TestLoadKeysPromptCanceled(t *testing.T) {
	r := runtest.NewRunner().On("ssh-add", agentEmpty()).On("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupFound: false}
	prompter := &fakePrompter{err: prompt.ErrCanceled}
	adder := &fakeKeyAdder{}
	notifier := &fakeNotifier{}
	log := &fakeLogger{}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: secret, Prompt: prompter, Adder: adder, Log: log,
		Notify: notifier, Config: Config{},
	}
	require.NoError(t, l.LoadKeys(t.Context()), "loading a login's keys must not fail")
	assert.Emptyf(t, adder.calls, "a question the user turned down leaves the key alone: %+v", adder.calls)
	assert.Truef(t, log.contains("dismissed"), "and the log must say so: %v", log.lines)
	// F38: turning the question down is an answer, so nothing in the session
	// log reads as something having gone wrong with the product.
	assertNothingWentWrong(t, log, "turning the question down is an answer, not a failure")
	assert.Emptyf(t, notifier.msgs, "and the user must not be told about a choice they just made: %v", notifier.msgs)
}

// TestLoadKeysDismissedDialogEndsTheAsking verifies F38: closing a passphrase
// dialog without answering ends the asking for the rest of that login, so
// shutting one window a user never asked for does not leave them with one more
// of them per key. Nothing is added, nobody is told of a failure, and no key is
// given up — the next login shell asks again from the first key.
func TestLoadKeysDismissedDialogEndsTheAsking(t *testing.T) {
	r := runtest.NewRunner().On("ssh-add", agentEmpty()).On("ssh-keygen", keygen("SHA256:NEW"))
	prompter := &fakePrompter{err: prompt.ErrCanceled}
	adder := &fakeKeyAdder{}
	notifier := &fakeNotifier{}
	give := newFakeGiveup()
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_one", "/ssh/id_two", "/ssh/id_three"}},
		Runner: r, Secret: &fakeSecret{}, Prompt: prompter, Adder: adder, Log: &fakeLogger{},
		Notify: notifier, Giveup: give, Config: Config{},
	}
	require.NoError(t, l.LoadKeys(t.Context()), "loading a login's keys must not fail")
	assert.Equal(t, []string{"id_one"}, prompter.calls,
		"shutting one window must not leave the user one more of them per key")
	assert.Emptyf(t, adder.calls, "nothing is loaded: %+v", adder.calls)
	assert.Emptyf(t, notifier.msgs, "nobody is told of a failure, because there was none: %v", notifier.msgs)
	assert.Emptyf(t, give.recorded,
		"and no key is given up: the next login shell asks again from the first one: %v", give.recorded)
}

// TestLoadKeysDismissedDialogSkipsOnlyThatKey verifies the "skip" half of F38:
// a user who would rather turn down one key and still be asked about the others
// says so, and is asked about every one of them.
func TestLoadKeysDismissedDialogSkipsOnlyThatKey(t *testing.T) {
	r := runtest.NewRunner().On("ssh-add", agentEmpty()).On("ssh-keygen", keygen("SHA256:NEW"))
	prompter := &fakePrompter{err: prompt.ErrCanceled}
	notifier := &fakeNotifier{}
	give := newFakeGiveup()
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_one", "/ssh/id_two", "/ssh/id_three"}},
		Runner: r, Secret: &fakeSecret{}, Prompt: prompter, Adder: &fakeKeyAdder{}, Log: &fakeLogger{},
		Notify: notifier, Giveup: give, Config: Config{OnDismiss: OnDismissSkip},
	}
	require.NoError(t, l.LoadKeys(t.Context()), "loading a login's keys must not fail")
	assert.Lenf(t, prompter.calls, 3,
		"a user who asked to skip turns down one key, not the rest: %v", prompter.calls)
	assert.Emptyf(t, give.recorded, "and no key is given up: %v", give.recorded)
	assert.Emptyf(t, notifier.msgs, "nor is anyone told of a failure, because there was none: %v", notifier.msgs)
}

// TestLoadKeysDismissedDialogCountsAsAWrongAnswer verifies the "retry" half of
// F38: the dismissal is treated as a wrong answer, so the same key is asked
// about until the attempts run out and it then ends the way F8 says a key that
// never opened ends — told once, and left alone.
func TestLoadKeysDismissedDialogCountsAsAWrongAnswer(t *testing.T) {
	r := runtest.NewRunner().On("ssh-add", agentEmpty()).On("ssh-keygen", keygen("SHA256:NEW"))
	prompter := &fakePrompter{err: prompt.ErrCanceled}
	notifier := &fakeNotifier{}
	give := newFakeGiveup()
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: &fakeSecret{}, Prompt: prompter, Adder: &fakeKeyAdder{}, Log: &fakeLogger{},
		Notify: notifier, Giveup: give, Config: Config{MaxAttempts: 3, OnDismiss: OnDismissRetry},
	}
	require.NoError(t, l.LoadKeys(t.Context()), "loading a login's keys must not fail")
	assert.Lenf(t, prompter.calls, 3,
		"a user who asked to retry is asked again until the attempts run out: %v", prompter.calls)
	assert.Equal(t, []string{"id_rsa"}, give.recorded, "then the key is given up, so the next login leaves it alone")
	require.Lenf(t, notifier.msgs, 1, "and they are told, once: %v", notifier.msgs)
	assert.Contains(t, notifier.msgs[0], "could not load key id_rsa", "with the key named")
}

// TestLoadKeysNoGUIStillUsesVault confirms the proactive loader consults the
// secret backend regardless of any graphical prompter being available — a
// headless interactive session with a CLI-only backend (op, bw) must still
// benefit from a stored passphrase, not just kdialog-equipped sessions.
func TestLoadKeysNoGUIStillUsesVault(t *testing.T) {
	r := runtest.NewRunner().On("ssh-add", agentEmpty()).On("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupPass: "stored-pass", lookupFound: true}
	adder := &fakeKeyAdder{withCodes: []int{0}}
	log := &fakeLogger{}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: secret, Prompt: &fakePrompter{}, Adder: adder, Log: log,
	}
	require.NoError(t, l.LoadKeys(t.Context()), "loading a login's keys must not fail")
	require.Lenf(t, adder.calls, 1, "the key must be loaded: %+v", adder.calls)
	assert.Equal(t, "stored-pass", adder.calls[0].passphrase,
		"out of the wallet: a session with no dialog on it still has a wallet a command-line tool can read")
}

// TestLoadKeysNoTerminalSkipsSilently confirms that having no controlling
// terminal to prompt on — a normal, expected condition for a non-interactive
// or otherwise detached invocation — never surfaces as a user-visible notice
// and is logged at INFO, not ERROR.
func TestLoadKeysNoTerminalSkipsSilently(t *testing.T) {
	r := runtest.NewRunner().On("ssh-add", agentEmpty()).On("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupFound: false}
	prompter := &fakePrompter{err: prompt.ErrNoTerminal}
	adder := &fakeKeyAdder{}
	notifier := &fakeNotifier{}
	log := &fakeLogger{}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: secret, Prompt: prompter, Adder: adder, Log: log,
		Notify: notifier,
	}
	require.NoError(t, l.LoadKeys(t.Context()), "loading a login's keys must not fail")
	assert.Emptyf(t, adder.calls, "with nowhere to ask, no key can be opened: %+v", adder.calls)
	assert.Truef(t, log.contains("INFO no terminal available"), "the log must say why: %v", log.lines)
	assertNothingWentWrong(t, log, "a detached invocation having no terminal is expected, not broken")
	assert.Emptyf(t, notifier.msgs,
		"and nobody is there to read a notice about it either: %v", notifier.msgs)
}

func TestLoadKeysSkipsGivenUp(t *testing.T) {
	r := runtest.NewRunner().On("ssh-add", agentEmpty()).On("ssh-keygen", keygen("SHA256:NEW"))
	adder := &fakeKeyAdder{}
	give := newFakeGiveup()
	give.given["id_rsa"] = true
	log := &fakeLogger{}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: &fakeSecret{}, Prompt: &fakePrompter{}, Adder: adder, Log: log,
		Giveup: give, Config: Config{},
	}
	require.NoError(t, l.LoadKeys(t.Context()), "loading a login's keys must not fail")
	assert.Emptyf(t, adder.calls,
		"a key given up on earlier must be left alone, or the user is asked about it every login: %+v", adder.calls)
	assert.Truef(t, log.contains("given up earlier"), "and the log must say why it was skipped: %v", log.lines)
}

func TestLoadKeysRecordsGiveupAfterRetries(t *testing.T) {
	r := runtest.NewRunner().On("ssh-add", agentEmpty()).On("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupPass: "stale", lookupFound: true}
	prompter := &fakePrompter{pass: "still-wrong"}
	adder := &fakeKeyAdder{withCodes: []int{1, 1, 1, 1}}
	give := newFakeGiveup()
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: secret, Prompt: prompter, Adder: adder, Log: &fakeLogger{},
		Giveup: give, Config: Config{},
	}
	require.NoError(t, l.LoadKeys(t.Context()), "loading a login's keys must not fail")
	assert.Equal(t, []string{"id_rsa"}, give.recorded,
		"a key the attempts ran out on must be given up, so the next login does not ask again")
}

func TestLoadKeysClearsGiveupOnSuccess(t *testing.T) {
	r := runtest.NewRunner().On("ssh-add", agentEmpty()).On("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupPass: "ok", lookupFound: true}
	adder := &fakeKeyAdder{withCodes: []int{0}}
	give := newFakeGiveup()
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: secret, Prompt: &fakePrompter{}, Adder: adder, Log: &fakeLogger{},
		Giveup: give, Config: Config{},
	}
	require.NoError(t, l.LoadKeys(t.Context()), "loading a login's keys must not fail")
	assert.Equal(t, []string{"id_rsa"}, give.cleared,
		"a key that opened must stop being given up on, or it is skipped for good")
}

func TestLoadKeysSavesKeyStateOnSuccess(t *testing.T) {
	r := runtest.NewRunner().On("ssh-add", agentEmpty()).On("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupPass: "ok", lookupFound: true}
	adder := &fakeKeyAdder{withCodes: []int{0}}
	ks := &fakeKeyState{}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: secret, Prompt: &fakePrompter{}, Adder: adder, Log: &fakeLogger{},
		KeyState: ks, Config: Config{KeyLifetime: 8 * time.Hour},
	}
	require.NoError(t, l.LoadKeys(t.Context()), "loading a login's keys must not fail")
	assert.Equal(t, []keyStateCall{{"id_rsa", 8 * time.Hour}}, ks.saved,
		"a key that opened must be recorded with how long it stays in the agent, or nothing knows when to refill it")
}

func TestLoadKeysSkipsLoadedNeverSavesKeyState(t *testing.T) {
	r := runtest.NewRunner().
		On("ssh-add", runtest.Stdout("256 SHA256:DUP loaded (ED25519)\n", 0)).
		On("ssh-keygen", keygen("SHA256:DUP"))
	ks := &fakeKeyState{}
	l := Loader{
		Keys:     fakeLister{paths: []string{"/ssh/id_x"}},
		Runner:   r,
		Adder:    &fakeKeyAdder{},
		Log:      &fakeLogger{},
		KeyState: ks,
		Config:   Config{},
	}
	require.NoError(t, l.LoadKeys(t.Context()), "loading a login's keys must not fail")
	assert.Emptyf(t, ks.saved,
		"a key already in the agent was not loaded now, and recording it would move an expiry nobody reset: %v", ks.saved)
}

func TestLoadKeysExhaustedRetriesNeverSavesKeyState(t *testing.T) {
	r := runtest.NewRunner().On("ssh-add", agentEmpty()).On("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupFound: false}
	prompter := &fakePrompter{pass: "wrong"}
	adder := &fakeKeyAdder{withCodes: []int{1, 1, 1}}
	ks := &fakeKeyState{}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: secret, Prompt: prompter, Adder: adder, Log: &fakeLogger{},
		Giveup: newFakeGiveup(), KeyState: ks, Config: Config{},
	}
	require.NoError(t, l.LoadKeys(t.Context()), "loading a login's keys must not fail")
	assert.Emptyf(t, ks.saved, "a key that never opened has no lifetime to record: %v", ks.saved)
}

func TestLoadKeysSessionUnlocksOnceAcrossKeys(t *testing.T) {
	r := runtest.NewRunner().On("ssh-add", agentEmpty()).On("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupPass: "stored-pass", lookupFound: true}
	adder := &fakeKeyAdder{withCodes: []int{0, 0}}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa", "/ssh/id_ed25519"}},
		Runner: r, Secret: secret, Prompt: &fakePrompter{}, Adder: adder, Log: &fakeLogger{},
		Config: Config{},
	}
	require.NoError(t, l.LoadKeys(t.Context()), "loading a login's keys must not fail")
	assert.Lenf(t, adder.calls, 2, "both keys must load: %+v", adder.calls)
	assert.Equal(t, 1, secret.unlockCalls,
		"one unlock covers the whole login: unlocking per key is a wallet dialog per key")
	assert.Equal(t, 1, secret.lockCalls, "and one lock closes it again when the login's keys are done")
}

func TestLoadKeysSessionSkipsUnlockWhenNothingNeeded(t *testing.T) {
	r := runtest.NewRunner().
		On("ssh-add", runtest.Stdout("256 SHA256:DUP loaded (ED25519)\n", 0)).
		On("ssh-keygen", keygen("SHA256:DUP"))
	secret := &fakeSecret{}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_x"}},
		Runner: r, Secret: secret, Adder: &fakeKeyAdder{}, Log: &fakeLogger{},
		Config: Config{},
	}
	require.NoError(t, l.LoadKeys(t.Context()), "loading a login's keys must not fail")
	assert.Zero(t, secret.unlockCalls,
		"every key was already in the agent, so opening the wallet would raise a dialog nothing needed")
	assert.Zero(t, secret.lockCalls, "and there is nothing to close")
}

func TestLoadKeysSessionUnlockFailureFallsBackPerKey(t *testing.T) {
	r := runtest.NewRunner().On("ssh-add", agentEmpty()).On("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupPass: "stored-pass", lookupFound: true, unlockErr: errors.New("dismissed")}
	adder := &fakeKeyAdder{withCodes: []int{0, 0}}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa", "/ssh/id_ed25519"}},
		Runner: r, Secret: secret, Prompt: &fakePrompter{}, Adder: adder, Log: &fakeLogger{},
		Config: Config{},
	}
	require.NoError(t, l.LoadKeys(t.Context()), "loading a login's keys must not fail")
	assert.Lenf(t, adder.calls, 2,
		"a wallet that would not open for the batch must not cost the user their keys: %+v", adder.calls)
	assert.Equal(t, 2, secret.unlockCalls, "each key tries for itself instead")
	assert.Zero(t, secret.lockCalls, "and nothing may be locked: the wallet was never open")
}

func TestLoadKeysNoKeys(t *testing.T) {
	r := runtest.NewRunner() // ssh-add must not be consulted
	log := &fakeLogger{}
	l := Loader{Keys: fakeLister{paths: nil}, Runner: r, Log: log}
	require.NoError(t, l.LoadKeys(t.Context()), "loading a login's keys must not fail")
	assert.Truef(t, log.contains("no keys"), "the log must say there was nothing to do: %v", log.lines)
	assert.Emptyf(t, r.Calls, "and a login with no keys must not go asking the agent about them: %v", r.Calls)
}

func TestLoadKeysEnumerateError(t *testing.T) {
	l := Loader{Keys: fakeLister{err: errors.New("readdir boom")}, Runner: runtest.NewRunner()}
	assert.Error(t, l.LoadKeys(t.Context()),
		"a key directory that could not be read must be reported, not read as a login with no keys")
}

func TestLoadKeysAgentSnapshotError(t *testing.T) {
	r := runtest.NewRunner().On("ssh-add", runtest.Fails(errors.New("no ssh-add")))
	l := Loader{Keys: fakeLister{paths: []string{"/ssh/id_rsa"}}, Runner: r}
	assert.Error(t, l.LoadKeys(t.Context()),
		"an agent that could not be asked what it holds must be reported: every key would look unloaded")
}
