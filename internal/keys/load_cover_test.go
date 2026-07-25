package keys

import (
	"errors"
	"testing"
)

// TestLoadKeysStoredAddErrorFailsAndNotifies covers failAdd on the stored-
// passphrase path: ssh-add cannot be run at all, which is logged at ERROR and
// surfaced to the user, and the key is abandoned without a give-up record.
func TestLoadKeysStoredAddErrorFailsAndNotifies(t *testing.T) {
	r := newFakeRunner().on("ssh-add", agentEmpty()).on("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupPass: "stored-pass", lookupFound: true}
	adder := &fakeKeyAdder{err: errors.New("ssh-add exec boom")}
	log := &fakeLogger{}
	notify := &fakeNotifier{}
	giveup := newFakeGiveup()
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: secret, Prompt: &fakePrompter{}, Adder: adder,
		Log: log, Notify: notify, Giveup: giveup,
	}
	if err := l.LoadKeys(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !log.contains("ERROR add id_rsa") {
		t.Fatalf("expected an ERROR add log, got %v", log.lines)
	}
	if len(notify.msgs) != 1 {
		t.Fatalf("expected one user notice, got %v", notify.msgs)
	}
	if len(giveup.recorded) != 0 {
		t.Fatalf("a hard add error must not record a give-up, got %v", giveup.recorded)
	}
}

// TestLoadKeysPromptAddErrorFailsAndNotifies covers failAdd on the prompt path:
// a passphrase is obtained but ssh-add cannot run.
func TestLoadKeysPromptAddErrorFailsAndNotifies(t *testing.T) {
	r := newFakeRunner().on("ssh-add", agentEmpty()).on("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupFound: false}
	prompter := &fakePrompter{pass: "typed-pass"}
	adder := &fakeKeyAdder{err: errors.New("ssh-add exec boom")}
	log := &fakeLogger{}
	notify := &fakeNotifier{}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: secret, Prompt: prompter, Adder: adder,
		Log: log, Notify: notify,
	}
	if err := l.LoadKeys(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !log.contains("ERROR add id_rsa") {
		t.Fatalf("expected an ERROR add log, got %v", log.lines)
	}
	if len(notify.msgs) != 1 {
		t.Fatalf("expected one user notice, got %v", notify.msgs)
	}
}

// TestLoadKeysPromptHardErrorFailsAndNotifies covers failPrompt: the prompter
// fails for a reason that is neither a cancel nor a missing terminal, which is
// an operator problem — logged at ERROR and surfaced to the user.
func TestLoadKeysPromptHardErrorFailsAndNotifies(t *testing.T) {
	r := newFakeRunner().on("ssh-add", agentEmpty()).on("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupFound: false}
	prompter := &fakePrompter{err: errors.New("kdialog crashed")}
	adder := &fakeKeyAdder{}
	log := &fakeLogger{}
	notify := &fakeNotifier{}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: secret, Prompt: prompter, Adder: adder,
		Log: log, Notify: notify,
	}
	if err := l.LoadKeys(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(adder.calls) != 0 {
		t.Fatalf("a hard prompt error must not attempt an add, got %v", adder.calls)
	}
	if !log.contains("ERROR prompt id_rsa") {
		t.Fatalf("expected an ERROR prompt log, got %v", log.lines)
	}
	if len(notify.msgs) != 1 {
		t.Fatalf("expected one user notice, got %v", notify.msgs)
	}
}

// TestLoadKeysStoreErrorLogged covers storePassphrase's store-failure branch:
// the key is already in the agent, so a failure to persist the passphrase is
// logged at ERROR but does not fail the load.
func TestLoadKeysStoreErrorLogged(t *testing.T) {
	r := newFakeRunner().on("ssh-add", agentEmpty()).on("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupFound: false, storeErr: errors.New("wallet write denied")}
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
	if !log.contains("ERROR store passphrase for id_rsa") {
		t.Fatalf("expected an ERROR store log, got %v", log.lines)
	}
}

// TestLoadKeysRecordGiveupErrorLogged covers recordGiveup's error branch: the
// retries are exhausted and persisting the give-up itself fails.
func TestLoadKeysRecordGiveupErrorLogged(t *testing.T) {
	r := newFakeRunner().on("ssh-add", agentEmpty()).on("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupFound: false}
	prompter := &fakePrompter{pass: "wrong"}
	adder := &fakeKeyAdder{withCodes: []int{1, 1, 1}}
	log := &fakeLogger{}
	giveup := newFakeGiveup()
	giveup.recordErr = errors.New("giveup store write denied")
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: secret, Prompt: prompter, Adder: adder, Log: log,
		Notify: &fakeNotifier{}, Giveup: giveup,
		Config: Config{MaxAttempts: 3},
	}
	if err := l.LoadKeys(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !log.contains("ERROR record give-up for id_rsa") {
		t.Fatalf("expected an ERROR record-give-up log, got %v", log.lines)
	}
}

// TestLoadKeysClearGiveupErrorLogged covers clearGiveup's error branch: the key
// loads, but dropping its stale give-up record fails.
func TestLoadKeysClearGiveupErrorLogged(t *testing.T) {
	r := newFakeRunner().on("ssh-add", agentEmpty()).on("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupPass: "stored-pass", lookupFound: true}
	adder := &fakeKeyAdder{withCodes: []int{0}}
	log := &fakeLogger{}
	giveup := newFakeGiveup()
	giveup.clearErr = errors.New("giveup store clear denied")
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: secret, Prompt: &fakePrompter{}, Adder: adder, Log: log,
		Giveup: giveup,
	}
	if err := l.LoadKeys(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !log.contains("ERROR clear give-up for id_rsa") {
		t.Fatalf("expected an ERROR clear-give-up log, got %v", log.lines)
	}
}

// TestLoadKeysSaveKeyStateErrorLogged covers saveKeyState's error branch: the
// key loads, but recording its agent lifetime fails.
func TestLoadKeysSaveKeyStateErrorLogged(t *testing.T) {
	r := newFakeRunner().on("ssh-add", agentEmpty()).on("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupPass: "stored-pass", lookupFound: true}
	adder := &fakeKeyAdder{withCodes: []int{0}}
	log := &fakeLogger{}
	l := Loader{
		Keys:     fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner:   r,
		Secret:   secret,
		Prompt:   &fakePrompter{},
		Adder:    adder,
		Log:      log,
		KeyState: &fakeKeyState{err: errors.New("keystate write denied")},
	}
	if err := l.LoadKeys(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !log.contains("ERROR record key state for id_rsa") {
		t.Fatalf("expected an ERROR key-state log, got %v", log.lines)
	}
}

// TestLoadKeysFingerprintErrorPressesOn covers loadOne's fingerprint-error
// branch: ssh-keygen cannot run, so dedup is impossible, but the loader logs
// the failure and still attempts to add the key.
func TestLoadKeysFingerprintErrorPressesOn(t *testing.T) {
	r := newFakeRunner().
		on("ssh-add", agentEmpty()).
		on("ssh-keygen", fails(errors.New("ssh-keygen not found")))
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
	if !log.contains("ERROR fingerprint id_rsa") {
		t.Fatalf("expected an ERROR fingerprint log, got %v", log.lines)
	}
	if len(adder.calls) != 1 {
		t.Fatalf("a fingerprint failure must still attempt the add, got %v", adder.calls)
	}
}

// TestLoadKeysSessionLockErrorLogged covers LoadKeys's batch-lock-failure
// branch: the wallet was unlocked for the batch, and re-locking it afterward
// fails.
func TestLoadKeysSessionLockErrorLogged(t *testing.T) {
	r := newFakeRunner().on("ssh-add", agentEmpty()).on("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupPass: "stored-pass", lookupFound: true, lockErr: errors.New("wallet lock denied")}
	adder := &fakeKeyAdder{withCodes: []int{0}}
	log := &fakeLogger{}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: secret, Prompt: &fakePrompter{}, Adder: adder, Log: log,
	}
	if err := l.LoadKeys(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if secret.lockCalls != 1 {
		t.Fatalf("expected exactly one batch Lock, got %d", secret.lockCalls)
	}
	if !log.contains("ERROR lock secret store") {
		t.Fatalf("expected an ERROR lock log, got %v", log.lines)
	}
}
