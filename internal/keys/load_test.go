package keys

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// agentEmpty answers `ssh-add -l` as an empty agent; keygen answers `ssh-keygen
// -lf` with a fingerprint line for a key file.
func agentEmpty() func(Cmd) (Result, error) { return stdout("The agent has no identities.\n", 1) }
func keygen(fp string) func(Cmd) (Result, error) {
	return stdout("256 "+fp+" comment (ED25519)\n", 0)
}

func TestLoadKeysSkipsLoaded(t *testing.T) {
	r := newFakeRunner().
		on("ssh-add", stdout("256 SHA256:DUP loaded (ED25519)\n", 0)).
		on("ssh-keygen", keygen("SHA256:DUP"))
	adder := &fakeKeyAdder{}
	log := &fakeLogger{}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_x"}},
		Runner: r,
		Adder:  adder,
		Log:    log,
		Config: Config{},
	}
	if err := l.LoadKeys(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(adder.calls) != 0 {
		t.Fatalf("a loaded key must not be added, got %d adds", len(adder.calls))
	}
	if !log.contains("already added") {
		t.Fatalf("expected an 'already added' log, got %v", log.lines)
	}
}

func TestLoadKeysStoredPassphrase(t *testing.T) {
	r := newFakeRunner().on("ssh-add", agentEmpty()).on("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupPass: "stored-pass", lookupFound: true}
	adder := &fakeKeyAdder{withCodes: []int{0}}
	log := &fakeLogger{}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: secret, Prompt: &fakePrompter{}, Adder: adder, Log: log,
		Config: Config{},
	}
	if err := l.LoadKeys(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(adder.calls) != 1 || adder.calls[0].passphrase != "stored-pass" {
		t.Fatalf("calls = %+v, want one askpass add with the stored pass", adder.calls)
	}
	if len(secret.stored) != 0 {
		t.Fatalf("a looked-up passphrase must not be re-stored, got %v", secret.stored)
	}
}

func TestLoadKeysPromptThenStore(t *testing.T) {
	r := newFakeRunner().on("ssh-add", agentEmpty()).on("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupFound: false}
	prompter := &fakePrompter{pass: "typed-pass"}
	adder := &fakeKeyAdder{withCodes: []int{0}}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: secret, Prompt: prompter, Adder: adder, Log: &fakeLogger{},
		Config: Config{},
	}
	if err := l.LoadKeys(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(adder.calls) != 1 || adder.calls[0].passphrase != "typed-pass" {
		t.Fatalf("calls = %+v, want one add with the prompted pass", adder.calls)
	}
	if len(secret.stored) != 1 {
		t.Fatalf("a prompted passphrase must be stored once, got %v", secret.stored)
	}
	got := secret.stored[0]
	if got.service != defaultServicePrefix+"-id_rsa" || got.label != "SSH Passphrase for id_rsa" || got.passphrase != "typed-pass" {
		t.Fatalf("store = %+v, want service/label/pass for id_rsa", got)
	}
}

// TestLoadKeysLookupErrorLogsInfoNotError confirms a lookup error (the
// backend being unreachable in this environment, e.g. no D-Bus session) is
// logged at INFO and still falls through to prompting — not treated as an
// operator-actionable ERROR the way a genuine failure later (a rejected
// stored passphrase, an exhausted retry loop) still is.
func TestLoadKeysLookupErrorLogsInfoNotError(t *testing.T) {
	r := newFakeRunner().on("ssh-add", agentEmpty()).on("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupErr: errors.New("dbus: not reachable")}
	prompter := &fakePrompter{pass: "typed-pass"}
	adder := &fakeKeyAdder{withCodes: []int{0}}
	log := &fakeLogger{}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: secret, Prompt: prompter, Adder: adder, Log: log,
	}
	if err := l.LoadKeys(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(adder.calls) != 1 || adder.calls[0].passphrase != "typed-pass" {
		t.Fatalf("calls = %+v, want a lookup error to fall through to prompting", adder.calls)
	}
	if !log.contains("INFO secret lookup") {
		t.Fatalf("expected an INFO secret-lookup log, got %v", log.lines)
	}
	for _, l := range log.lines {
		if strings.HasPrefix(l, "ERROR") {
			t.Fatalf("an unreachable backend must not log at ERROR, got %v", log.lines)
		}
	}
}

func TestLoadKeysPromptThenStoreExcludedByPolicy(t *testing.T) {
	r := newFakeRunner().on("ssh-add", agentEmpty()).on("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupFound: false}
	prompter := &fakePrompter{pass: "typed-pass"}
	adder := &fakeKeyAdder{withCodes: []int{0}}
	log := &fakeLogger{}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: secret, Prompt: prompter, Adder: adder, Log: log,
		Config: Config{WalletStore: func(keyname string) bool { return keyname != "id_rsa" }},
	}
	if err := l.LoadKeys(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(adder.calls) != 1 || adder.calls[0].passphrase != "typed-pass" {
		t.Fatalf("calls = %+v, want one add with the prompted pass", adder.calls)
	}
	if len(secret.stored) != 0 {
		t.Fatalf("an excluded key must not be stored, got %v", secret.stored)
	}
	if !log.contains("wallet-store policy excludes id_rsa") {
		t.Fatalf("expected an excluded-by-policy log, got %v", log.lines)
	}
}

func TestLoadKeysAutoLoadExcludedByPolicyNeverAdded(t *testing.T) {
	// ssh-keygen is deliberately not registered: an excluded key must never
	// reach the fingerprint lookup, only the agent snapshot (ssh-add -l).
	r := newFakeRunner().on("ssh-add", agentEmpty())
	adder := &fakeKeyAdder{}
	log := &fakeLogger{}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Adder: adder, Log: log,
		Config: Config{AutoLoad: func(keyname string) bool { return keyname != "id_rsa" }},
	}
	if err := l.LoadKeys(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(adder.calls) != 0 {
		t.Fatalf("an auto-load-excluded key must not be added, got %d adds", len(adder.calls))
	}
	if len(r.calls) != 1 || r.calls[0].Name != "ssh-add" {
		t.Fatalf("an auto-load-excluded key must not be fingerprinted, got calls %v", r.calls)
	}
	if !log.contains("auto-load policy excludes id_rsa") {
		t.Fatalf("expected an excluded-by-policy log, got %v", log.lines)
	}
}

func TestLoadKeysRetriesThenGivesUp(t *testing.T) {
	r := newFakeRunner().on("ssh-add", agentEmpty()).on("ssh-keygen", keygen("SHA256:NEW"))
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
	if err := l.LoadKeys(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(adder.calls) != 4 {
		t.Fatalf("want 1 stored + 3 prompted attempts, got %d", len(adder.calls))
	}
	if !log.contains("attempt 3/3") || !log.contains("giving up") {
		t.Fatalf("expected final-attempt and give-up logs, got %v", log.lines)
	}
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
	r := newFakeRunner().on("ssh-add", agentEmpty()).on("ssh-keygen", keygen("SHA256:NEW"))
	prompter := &fakePrompter{pass: ""}
	adder := &fakeKeyAdder{}
	log := &fakeLogger{}
	notes := &fakeNotifier{}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: &fakeSecret{}, Prompt: prompter, Adder: adder, Log: log,
		Notify: notes, Config: Config{MaxAttempts: 3},
	}
	if err := l.LoadKeys(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prompter.calls) != 3 {
		t.Errorf("asked %d times, want the configured 3", len(prompter.calls))
	}
	for _, c := range adder.calls {
		if c.passphrase == "" {
			t.Errorf("an empty passphrase reached the key adder: %+v", adder.calls)
			break
		}
	}
	if !log.contains("giving up") {
		t.Errorf("expected a give-up log after the attempts ran out, got %v", log.lines)
	}
	if len(notes.msgs) != 1 {
		t.Errorf("notices = %v, want the one give-up notice", notes.msgs)
	}
}

func TestLoadKeysStaleStoredThenPromptStores(t *testing.T) {
	r := newFakeRunner().on("ssh-add", agentEmpty()).on("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupPass: "stale", lookupFound: true}
	prompter := &fakePrompter{pass: "fresh"}
	adder := &fakeKeyAdder{withCodes: []int{1, 0}} // stored rejected, prompted accepted
	log := &fakeLogger{}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: secret, Prompt: prompter, Adder: adder, Log: log,
		Config: Config{},
	}
	if err := l.LoadKeys(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(adder.calls) != 2 || adder.calls[0].passphrase != "stale" || adder.calls[1].passphrase != "fresh" {
		t.Fatalf("calls = %+v, want a stale then a fresh add", adder.calls)
	}
	if len(secret.stored) != 1 || secret.stored[0].passphrase != "fresh" {
		t.Fatalf("the fresh passphrase must replace the stale one, got %v", secret.stored)
	}
	if !log.contains("is stale") {
		t.Fatalf("expected a stale-passphrase log, got %v", log.lines)
	}
}

func TestLoadKeysNotifiesOnGiveup(t *testing.T) {
	r := newFakeRunner().on("ssh-add", agentEmpty()).on("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupFound: false}
	prompter := &fakePrompter{pass: "wrong"}
	adder := &fakeKeyAdder{withCodes: []int{1, 1, 1}}
	notifier := &fakeNotifier{}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: secret, Prompt: prompter, Adder: adder, Log: &fakeLogger{},
		Notify: notifier, Config: Config{},
	}
	if err := l.LoadKeys(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(notifier.msgs) != 1 || !strings.Contains(notifier.msgs[0], "could not load key id_rsa") {
		t.Fatalf("expected one give-up notice, got %v", notifier.msgs)
	}
}

func TestLoadKeysPromptCanceled(t *testing.T) {
	r := newFakeRunner().on("ssh-add", agentEmpty()).on("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupFound: false}
	prompter := &fakePrompter{err: ErrPromptCanceled}
	adder := &fakeKeyAdder{}
	notifier := &fakeNotifier{}
	log := &fakeLogger{}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: secret, Prompt: prompter, Adder: adder, Log: log,
		Notify: notifier, Config: Config{},
	}
	if err := l.LoadKeys(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(adder.calls) != 0 {
		t.Fatalf("a canceled prompt must not add, got %d", len(adder.calls))
	}
	if !log.contains("dismissed") {
		t.Fatalf("expected a dismissed log, got %v", log.lines)
	}
	// F38: turning the question down is an answer, so nothing in the session
	// log reads as something having gone wrong with the product.
	for _, line := range log.lines {
		if strings.HasPrefix(line, "ERROR") {
			t.Fatalf("a dismissed prompt must never log at ERROR, got %v", log.lines)
		}
	}
	if len(notifier.msgs) != 0 {
		t.Fatalf("a dismissed prompt must not notify, got %v", notifier.msgs)
	}
}

// TestLoadKeysDismissedDialogEndsTheAsking verifies F38: closing a passphrase
// dialog without answering ends the asking for the rest of that login, so
// shutting one window a user never asked for does not leave them with one more
// of them per key. Nothing is added, nobody is told of a failure, and no key is
// given up — the next login shell asks again from the first key.
func TestLoadKeysDismissedDialogEndsTheAsking(t *testing.T) {
	r := newFakeRunner().on("ssh-add", agentEmpty()).on("ssh-keygen", keygen("SHA256:NEW"))
	prompter := &fakePrompter{err: ErrPromptCanceled}
	adder := &fakeKeyAdder{}
	notifier := &fakeNotifier{}
	give := newFakeGiveup()
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_one", "/ssh/id_two", "/ssh/id_three"}},
		Runner: r, Secret: &fakeSecret{}, Prompt: prompter, Adder: adder, Log: &fakeLogger{},
		Notify: notifier, Giveup: give, Config: Config{},
	}
	if err := l.LoadKeys(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prompter.calls) != 1 || prompter.calls[0] != "id_one" {
		t.Fatalf("asked for %v, want id_one alone: a dismissed dialog ends the asking", prompter.calls)
	}
	if len(adder.calls) != 0 {
		t.Fatalf("a dismissed dialog must not add, got %d", len(adder.calls))
	}
	if len(notifier.msgs) != 0 {
		t.Fatalf("a dismissed dialog is not a failure and must not notify, got %v", notifier.msgs)
	}
	if len(give.recorded) != 0 {
		t.Fatalf("a dismissed dialog must give up on no key, got %v", give.recorded)
	}
}

// TestLoadKeysDismissedDialogSkipsOnlyThatKey verifies the "skip" half of F38:
// a user who would rather turn down one key and still be asked about the others
// says so, and is asked about every one of them.
func TestLoadKeysDismissedDialogSkipsOnlyThatKey(t *testing.T) {
	r := newFakeRunner().on("ssh-add", agentEmpty()).on("ssh-keygen", keygen("SHA256:NEW"))
	prompter := &fakePrompter{err: ErrPromptCanceled}
	notifier := &fakeNotifier{}
	give := newFakeGiveup()
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_one", "/ssh/id_two", "/ssh/id_three"}},
		Runner: r, Secret: &fakeSecret{}, Prompt: prompter, Adder: &fakeKeyAdder{}, Log: &fakeLogger{},
		Notify: notifier, Giveup: give, Config: Config{OnDismiss: OnDismissSkip},
	}
	if err := l.LoadKeys(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prompter.calls) != 3 {
		t.Fatalf("asked for %v, want all three: skip turns down one key, not the rest", prompter.calls)
	}
	if len(give.recorded) != 0 {
		t.Fatalf("a dismissed dialog must give up on no key, got %v", give.recorded)
	}
	if len(notifier.msgs) != 0 {
		t.Fatalf("a dismissed dialog is not a failure and must not notify, got %v", notifier.msgs)
	}
}

// TestLoadKeysDismissedDialogCountsAsAWrongAnswer verifies the "retry" half of
// F38: the dismissal is treated as a wrong answer, so the same key is asked
// about until the attempts run out and it then ends the way F8 says a key that
// never opened ends — told once, and left alone.
func TestLoadKeysDismissedDialogCountsAsAWrongAnswer(t *testing.T) {
	r := newFakeRunner().on("ssh-add", agentEmpty()).on("ssh-keygen", keygen("SHA256:NEW"))
	prompter := &fakePrompter{err: ErrPromptCanceled}
	notifier := &fakeNotifier{}
	give := newFakeGiveup()
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: &fakeSecret{}, Prompt: prompter, Adder: &fakeKeyAdder{}, Log: &fakeLogger{},
		Notify: notifier, Giveup: give, Config: Config{MaxAttempts: 3, OnDismiss: OnDismissRetry},
	}
	if err := l.LoadKeys(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prompter.calls) != 3 {
		t.Fatalf("asked %d times, want 3: a dismissal counts as a wrong answer", len(prompter.calls))
	}
	if len(give.recorded) != 1 || give.recorded[0] != "id_rsa" {
		t.Fatalf("recorded = %v, want id_rsa given up once the attempts ran out", give.recorded)
	}
	if len(notifier.msgs) != 1 || !strings.Contains(notifier.msgs[0], "could not load key id_rsa") {
		t.Fatalf("expected one give-up notice, got %v", notifier.msgs)
	}
}

// TestLoadKeysNoGUIStillUsesVault confirms the proactive loader consults the
// secret backend regardless of any graphical prompter being available — a
// headless interactive session with a CLI-only backend (op, bw) must still
// benefit from a stored passphrase, not just kdialog-equipped sessions.
func TestLoadKeysNoGUIStillUsesVault(t *testing.T) {
	r := newFakeRunner().on("ssh-add", agentEmpty()).on("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupPass: "stored-pass", lookupFound: true}
	adder := &fakeKeyAdder{withCodes: []int{0}}
	log := &fakeLogger{}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: secret, Prompt: &fakePrompter{}, Adder: adder, Log: log,
	}
	if err := l.LoadKeys(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(adder.calls) != 1 || adder.calls[0].passphrase != "stored-pass" {
		t.Fatalf("calls = %+v, want one add with the stored pass, no GUI needed", adder.calls)
	}
}

// TestLoadKeysNoTerminalSkipsSilently confirms that having no controlling
// terminal to prompt on — a normal, expected condition for a non-interactive
// or otherwise detached invocation — never surfaces as a user-visible notice
// and is logged at INFO, not ERROR.
func TestLoadKeysNoTerminalSkipsSilently(t *testing.T) {
	r := newFakeRunner().on("ssh-add", agentEmpty()).on("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupFound: false}
	prompter := &fakePrompter{err: ErrNoTerminal}
	adder := &fakeKeyAdder{}
	notifier := &fakeNotifier{}
	log := &fakeLogger{}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: secret, Prompt: prompter, Adder: adder, Log: log,
		Notify: notifier,
	}
	if err := l.LoadKeys(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(adder.calls) != 0 {
		t.Fatalf("no terminal to prompt on must not add, got %d", len(adder.calls))
	}
	if !log.contains("INFO no terminal available") {
		t.Fatalf("expected an INFO no-terminal log, got %v", log.lines)
	}
	for _, l := range log.lines {
		if strings.HasPrefix(l, "ERROR") {
			t.Fatalf("no terminal available must never log at ERROR, got %v", log.lines)
		}
	}
	if len(notifier.msgs) != 0 {
		t.Fatalf("no terminal available must never notify the user, got %v", notifier.msgs)
	}
}

func TestLoadKeysSkipsGivenUp(t *testing.T) {
	r := newFakeRunner().on("ssh-add", agentEmpty()).on("ssh-keygen", keygen("SHA256:NEW"))
	adder := &fakeKeyAdder{}
	give := newFakeGiveup()
	give.given["id_rsa"] = true
	log := &fakeLogger{}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: &fakeSecret{}, Prompt: &fakePrompter{}, Adder: adder, Log: log,
		Giveup: give, Config: Config{},
	}
	if err := l.LoadKeys(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(adder.calls) != 0 {
		t.Fatalf("a given-up key must not be added, got %d adds", len(adder.calls))
	}
	if !log.contains("given up earlier") {
		t.Fatalf("expected a skip log, got %v", log.lines)
	}
}

func TestLoadKeysRecordsGiveupAfterRetries(t *testing.T) {
	r := newFakeRunner().on("ssh-add", agentEmpty()).on("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupPass: "stale", lookupFound: true}
	prompter := &fakePrompter{pass: "still-wrong"}
	adder := &fakeKeyAdder{withCodes: []int{1, 1, 1, 1}}
	give := newFakeGiveup()
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: secret, Prompt: prompter, Adder: adder, Log: &fakeLogger{},
		Giveup: give, Config: Config{},
	}
	if err := l.LoadKeys(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(give.recorded) != 1 || give.recorded[0] != "id_rsa" {
		t.Fatalf("recorded = %v, want [id_rsa]", give.recorded)
	}
}

func TestLoadKeysClearsGiveupOnSuccess(t *testing.T) {
	r := newFakeRunner().on("ssh-add", agentEmpty()).on("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupPass: "ok", lookupFound: true}
	adder := &fakeKeyAdder{withCodes: []int{0}}
	give := newFakeGiveup()
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: secret, Prompt: &fakePrompter{}, Adder: adder, Log: &fakeLogger{},
		Giveup: give, Config: Config{},
	}
	if err := l.LoadKeys(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(give.cleared) != 1 || give.cleared[0] != "id_rsa" {
		t.Fatalf("cleared = %v, want [id_rsa]", give.cleared)
	}
}

func TestLoadKeysSavesKeyStateOnSuccess(t *testing.T) {
	r := newFakeRunner().on("ssh-add", agentEmpty()).on("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupPass: "ok", lookupFound: true}
	adder := &fakeKeyAdder{withCodes: []int{0}}
	ks := &fakeKeyState{}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: secret, Prompt: &fakePrompter{}, Adder: adder, Log: &fakeLogger{},
		KeyState: ks, Config: Config{KeyLifetime: 8 * time.Hour},
	}
	if err := l.LoadKeys(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ks.saved) != 1 || ks.saved[0] != (keyStateCall{"id_rsa", 8 * time.Hour}) {
		t.Fatalf("saved = %v, want [{id_rsa 8h}]", ks.saved)
	}
}

func TestLoadKeysSkipsLoadedNeverSavesKeyState(t *testing.T) {
	r := newFakeRunner().
		on("ssh-add", stdout("256 SHA256:DUP loaded (ED25519)\n", 0)).
		on("ssh-keygen", keygen("SHA256:DUP"))
	ks := &fakeKeyState{}
	l := Loader{
		Keys:     fakeLister{paths: []string{"/ssh/id_x"}},
		Runner:   r,
		Adder:    &fakeKeyAdder{},
		Log:      &fakeLogger{},
		KeyState: ks,
		Config:   Config{},
	}
	if err := l.LoadKeys(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ks.saved) != 0 {
		t.Fatalf("saved = %v, want none for an already-loaded key", ks.saved)
	}
}

func TestLoadKeysExhaustedRetriesNeverSavesKeyState(t *testing.T) {
	r := newFakeRunner().on("ssh-add", agentEmpty()).on("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupFound: false}
	prompter := &fakePrompter{pass: "wrong"}
	adder := &fakeKeyAdder{withCodes: []int{1, 1, 1}}
	ks := &fakeKeyState{}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: secret, Prompt: prompter, Adder: adder, Log: &fakeLogger{},
		Giveup: newFakeGiveup(), KeyState: ks, Config: Config{},
	}
	if err := l.LoadKeys(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ks.saved) != 0 {
		t.Fatalf("saved = %v, want none after exhausted retries", ks.saved)
	}
}

func TestLoadKeysSessionUnlocksOnceAcrossKeys(t *testing.T) {
	r := newFakeRunner().on("ssh-add", agentEmpty()).on("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupPass: "stored-pass", lookupFound: true}
	adder := &fakeKeyAdder{withCodes: []int{0, 0}}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa", "/ssh/id_ed25519"}},
		Runner: r, Secret: secret, Prompt: &fakePrompter{}, Adder: adder, Log: &fakeLogger{},
		Config: Config{},
	}
	if err := l.LoadKeys(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(adder.calls) != 2 {
		t.Fatalf("want 2 adds, got %d", len(adder.calls))
	}
	if secret.unlockCalls != 1 || secret.lockCalls != 1 {
		t.Fatalf("unlockCalls=%d lockCalls=%d, want exactly one unlock and one lock for the whole batch",
			secret.unlockCalls, secret.lockCalls)
	}
}

func TestLoadKeysSessionSkipsUnlockWhenNothingNeeded(t *testing.T) {
	r := newFakeRunner().
		on("ssh-add", stdout("256 SHA256:DUP loaded (ED25519)\n", 0)).
		on("ssh-keygen", keygen("SHA256:DUP"))
	secret := &fakeSecret{}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_x"}},
		Runner: r, Secret: secret, Adder: &fakeKeyAdder{}, Log: &fakeLogger{},
		Config: Config{},
	}
	if err := l.LoadKeys(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if secret.unlockCalls != 0 || secret.lockCalls != 0 {
		t.Fatalf("unlockCalls=%d lockCalls=%d, want none: no key needed the wallet",
			secret.unlockCalls, secret.lockCalls)
	}
}

func TestLoadKeysSessionUnlockFailureFallsBackPerKey(t *testing.T) {
	r := newFakeRunner().on("ssh-add", agentEmpty()).on("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupPass: "stored-pass", lookupFound: true, unlockErr: errors.New("dismissed")}
	adder := &fakeKeyAdder{withCodes: []int{0, 0}}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa", "/ssh/id_ed25519"}},
		Runner: r, Secret: secret, Prompt: &fakePrompter{}, Adder: adder, Log: &fakeLogger{},
		Config: Config{},
	}
	if err := l.LoadKeys(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(adder.calls) != 2 {
		t.Fatalf("keys must still load despite the unlock failure, got %d adds", len(adder.calls))
	}
	if secret.unlockCalls != 2 {
		t.Fatalf("unlockCalls = %d, want a retry attempt per key when the session unlock keeps failing",
			secret.unlockCalls)
	}
	if secret.lockCalls != 0 {
		t.Fatalf("lockCalls = %d, want none: the session was never actually held", secret.lockCalls)
	}
}

func TestLoadKeysNoKeys(t *testing.T) {
	r := newFakeRunner() // ssh-add must not be consulted
	log := &fakeLogger{}
	l := Loader{Keys: fakeLister{paths: nil}, Runner: r, Log: log}
	if err := l.LoadKeys(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !log.contains("no keys") {
		t.Fatalf("expected a no-keys log, got %v", log.lines)
	}
	if len(r.calls) != 0 {
		t.Fatalf("the agent must not be queried with no keys, got %v", r.calls)
	}
}

func TestLoadKeysEnumerateError(t *testing.T) {
	l := Loader{Keys: fakeLister{err: errors.New("readdir boom")}, Runner: newFakeRunner()}
	if err := l.LoadKeys(); err == nil {
		t.Fatal("expected an error when enumeration fails")
	}
}

func TestLoadKeysAgentSnapshotError(t *testing.T) {
	r := newFakeRunner().on("ssh-add", fails(errors.New("no ssh-add")))
	l := Loader{Keys: fakeLister{paths: []string{"/ssh/id_rsa"}}, Runner: r}
	if err := l.LoadKeys(); err == nil {
		t.Fatal("expected an error when the agent snapshot fails")
	}
}
