package keys

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/run/runtest"
)

// TestLoadKeysStoredAddErrorFailsAndNotifies covers failAdd on the stored-
// passphrase path: ssh-add cannot be run at all, which is logged at ERROR and
// surfaced to the user, and the key is abandoned without a give-up record.
func TestLoadKeysStoredAddErrorFailsAndNotifies(t *testing.T) {
	r := runtest.NewRunner().On("ssh-add", agentEmpty()).On("ssh-keygen", keygen("SHA256:NEW"))
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
	require.NoError(t, l.LoadKeys(t.Context()), "one key that could not be loaded must not fail the whole login")
	assert.Truef(t, log.contains("ERROR add id_rsa"),
		"ssh-add not running at all is something an operator has to fix, and the log must say so: %v", log.lines)
	assert.Lenf(t, notify.msgs, 1, "and the user must be told their key is not loaded: %v", notify.msgs)
	assert.Emptyf(t, giveup.recorded,
		"but the key must not be given up on: nothing was learned about its passphrase: %v", giveup.recorded)
}

// TestLoadKeysPromptAddErrorFailsAndNotifies covers failAdd on the prompt path:
// a passphrase is obtained but ssh-add cannot run.
func TestLoadKeysPromptAddErrorFailsAndNotifies(t *testing.T) {
	r := runtest.NewRunner().On("ssh-add", agentEmpty()).On("ssh-keygen", keygen("SHA256:NEW"))
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
	require.NoError(t, l.LoadKeys(t.Context()), "one key that could not be loaded must not fail the whole login")
	assert.Truef(t, log.contains("ERROR add id_rsa"),
		"ssh-add not running at all is something an operator has to fix, and the log must say so: %v", log.lines)
	assert.Lenf(t, notify.msgs, 1, "and the user must be told their key is not loaded: %v", notify.msgs)
}

// TestLoadKeysPromptHardErrorFailsAndNotifies covers failPrompt: the prompter
// fails for a reason that is neither a cancel nor a missing terminal, which is
// an operator problem — logged at ERROR and surfaced to the user.
func TestLoadKeysPromptHardErrorFailsAndNotifies(t *testing.T) {
	r := runtest.NewRunner().On("ssh-add", agentEmpty()).On("ssh-keygen", keygen("SHA256:NEW"))
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
	require.NoError(t, l.LoadKeys(t.Context()), "one key that could not be loaded must not fail the whole login")
	assert.Emptyf(t, adder.calls, "with no passphrase in hand there is nothing to try: %+v", adder.calls)
	assert.Truef(t, log.contains("ERROR prompt id_rsa"),
		"a dialog that crashed is neither a dismissal nor a missing terminal, and the log must say so: %v", log.lines)
	assert.Lenf(t, notify.msgs, 1, "and the user must be told their key is not loaded: %v", notify.msgs)
}

// TestLoadKeysStoreErrorLogged covers storePassphrase's store-failure branch:
// the key is already in the agent, so a failure to persist the passphrase is
// logged at ERROR but does not fail the load.
func TestLoadKeysStoreErrorLogged(t *testing.T) {
	r := runtest.NewRunner().On("ssh-add", agentEmpty()).On("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupFound: false, storeErr: errors.New("wallet write denied")}
	prompter := &fakePrompter{pass: "typed-pass"}
	adder := &fakeKeyAdder{withCodes: []int{0}}
	log := &fakeLogger{}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: secret, Prompt: prompter, Adder: adder, Log: log,
	}
	require.NoError(t, l.LoadKeys(t.Context()), "one key that could not be loaded must not fail the whole login")
	assert.Truef(t, log.contains("ERROR store passphrase for id_rsa"),
		"a wallet that would not take the passphrase must be said so: the key is loaded, but the next login asks again: %v",
		log.lines)
}

// TestLoadKeysRecordGiveupErrorLogged covers recordGiveup's error branch: the
// retries are exhausted and persisting the give-up itself fails.
func TestLoadKeysRecordGiveupErrorLogged(t *testing.T) {
	r := runtest.NewRunner().On("ssh-add", agentEmpty()).On("ssh-keygen", keygen("SHA256:NEW"))
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
	require.NoError(t, l.LoadKeys(t.Context()), "one key that could not be loaded must not fail the whole login")
	assert.Truef(t, log.contains("ERROR record give-up for id_rsa"),
		"a give-up that could not be written down must be said so, or the next login asks about the key again: %v",
		log.lines)
}

// TestLoadKeysClearGiveupErrorLogged covers clearGiveup's error branch: the key
// loads, but dropping its stale give-up record fails.
func TestLoadKeysClearGiveupErrorLogged(t *testing.T) {
	r := runtest.NewRunner().On("ssh-add", agentEmpty()).On("ssh-keygen", keygen("SHA256:NEW"))
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
	require.NoError(t, l.LoadKeys(t.Context()), "one key that could not be loaded must not fail the whole login")
	assert.Truef(t, log.contains("ERROR clear give-up for id_rsa"),
		"a stale give-up that could not be dropped must be said so, or the key stays skipped though it opens: %v",
		log.lines)
}

// TestLoadKeysSaveKeyStateErrorLogged covers saveKeyState's error branch: the
// key loads, but recording its agent lifetime fails.
func TestLoadKeysSaveKeyStateErrorLogged(t *testing.T) {
	r := runtest.NewRunner().On("ssh-add", agentEmpty()).On("ssh-keygen", keygen("SHA256:NEW"))
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
	require.NoError(t, l.LoadKeys(t.Context()), "one key that could not be loaded must not fail the whole login")
	assert.Truef(t, log.contains("ERROR record key state for id_rsa"),
		"a lifetime that could not be recorded must be said so: nothing then knows when to refill the key: %v",
		log.lines)
}

// TestLoadKeysFingerprintErrorPressesOn covers loadOne's fingerprint-error
// branch: ssh-keygen cannot run, so dedup is impossible, but the loader logs
// the failure and still attempts to add the key.
func TestLoadKeysFingerprintErrorPressesOn(t *testing.T) {
	r := runtest.NewRunner().
		On("ssh-add", agentEmpty()).
		On("ssh-keygen", runtest.Fails(errors.New("ssh-keygen not found")))
	secret := &fakeSecret{lookupPass: "stored-pass", lookupFound: true}
	adder := &fakeKeyAdder{withCodes: []int{0}}
	log := &fakeLogger{}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: secret, Prompt: &fakePrompter{}, Adder: adder, Log: log,
	}
	require.NoError(t, l.LoadKeys(t.Context()), "one key that could not be loaded must not fail the whole login")
	assert.Truef(t, log.contains("ERROR fingerprint id_rsa"),
		"ssh-keygen not running at all is something an operator has to fix, and the log must say so: %v", log.lines)
	assert.Lenf(t, adder.calls, 1,
		"but not knowing whether the agent already holds the key is no reason to leave it out: %+v", adder.calls)
}

// TestLoadKeysSessionLockErrorLogged covers LoadKeys's batch-lock-failure
// branch: the wallet was unlocked for the batch, and re-locking it afterward
// fails.
func TestLoadKeysSessionLockErrorLogged(t *testing.T) {
	r := runtest.NewRunner().On("ssh-add", agentEmpty()).On("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupPass: "stored-pass", lookupFound: true, lockErr: errors.New("wallet lock denied")}
	adder := &fakeKeyAdder{withCodes: []int{0}}
	log := &fakeLogger{}
	l := Loader{
		Keys:   fakeLister{paths: []string{"/ssh/id_rsa"}},
		Runner: r, Secret: secret, Prompt: &fakePrompter{}, Adder: adder, Log: log,
	}
	require.NoError(t, l.LoadKeys(t.Context()), "one key that could not be loaded must not fail the whole login")
	assert.Equal(t, 1, secret.lockCalls, "the wallet opened for the batch must be closed once when the batch is done")
	assert.Truef(t, log.contains("ERROR lock secret store"),
		"and a wallet that would not close must be said so: it is left open with the user's secrets in it: %v",
		log.lines)
}

// TestLogfWithoutSinkIsNoop covers logf's guard for a Loader configured with no
// Log sink: the line is silently dropped rather than dereferencing a nil logger.
func TestLogfWithoutSinkIsNoop(t *testing.T) {
	(Loader{}).logf("INFO", "loaded %d keys", 3)
}
