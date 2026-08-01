package keys

import (
	"errors"
	"testing"
)

// TestParsePassphrasePromptWhitespaceKeyfile covers the empty-keyfile guard:
// a prompt whose captured path is only whitespace is not treated as a real
// key-passphrase request.
func TestParsePassphrasePromptWhitespaceKeyfile(t *testing.T) {
	if key, ok := ParsePassphrasePrompt("Enter passphrase for   :"); ok {
		t.Errorf("ParsePassphrasePrompt = (%q, true), want ok=false for a blank key path", key)
	}
}

// TestBrokerNonPassphraseNoTerminalLogsInfo covers the non-passphrase prompt
// path when there is no terminal: a normal, expected condition logged at INFO,
// answered with a declined (ok=false) reply.
func TestBrokerNonPassphraseNoTerminalLogsInfo(t *testing.T) {
	log := &fakeLogger{}
	tty := &fakeTTY{err: ErrNoTerminal}
	b := Broker{Secret: &fakeSecret{}, TTY: tty, Log: log}

	reply, ok := b.Answer("Please enter your login password:")
	if ok || reply != "" {
		t.Fatalf("Answer = (%q, %v), want (\"\", false)", reply, ok)
	}
	if !log.contains("INFO askpass: no terminal for prompt") {
		t.Fatalf("expected an INFO no-terminal log, got %v", log.lines)
	}
}

// TestBrokerNonPassphraseHardErrorLogsError covers the non-passphrase prompt
// path when prompting fails for an operator-actionable reason.
func TestBrokerNonPassphraseHardErrorLogsError(t *testing.T) {
	log := &fakeLogger{}
	tty := &fakeTTY{err: errors.New("terminal ioctl boom")}
	b := Broker{Secret: &fakeSecret{}, TTY: tty, Log: log}

	reply, ok := b.Answer("Please enter your login password:")
	if ok || reply != "" {
		t.Fatalf("Answer = (%q, %v), want (\"\", false)", reply, ok)
	}
	if !log.contains("ERROR askpass: no terminal for prompt") {
		t.Fatalf("expected an ERROR no-terminal log, got %v", log.lines)
	}
}

// TestBrokerStoreErrorLogged covers storePassphrase's store-failure branch:
// after a wallet miss the user types a passphrase, but persisting it fails —
// logged at ERROR while the typed value is still returned to ssh.
func TestBrokerStoreErrorLogged(t *testing.T) {
	log := &fakeLogger{}
	secret := &fakeSecret{lookupFound: false, storeErr: errors.New("wallet write denied")}
	tty := &fakeTTY{answer: "typed-pass"}
	b := Broker{Secret: secret, TTY: tty, Log: log}

	reply, ok := b.Answer("Enter passphrase for /home/u/.ssh/id_rsa:")
	if !ok || reply != "typed-pass" {
		t.Fatalf("Answer = (%q, %v), want (\"typed-pass\", true)", reply, ok)
	}
	if !log.contains("ERROR askpass: store passphrase for id_rsa") {
		t.Fatalf("expected an ERROR store log, got %v", log.lines)
	}
}

// TestBrokerNilLoggerDoesNotPanic covers logf's nil-Logger guard: a Broker with
// no Logger answers normally without logging.
func TestBrokerNilLoggerDoesNotPanic(t *testing.T) {
	secret := &fakeSecret{lookupPass: "stored", lookupFound: true}
	b := Broker{Secret: secret, TTY: &fakeTTY{}, Log: nil}

	reply, ok := b.Answer("Enter passphrase for /home/u/.ssh/id_rsa:")
	if !ok || reply != "stored" {
		t.Fatalf("Answer = (%q, %v), want (\"stored\", true)", reply, ok)
	}
}

// TestBrokerCustomServicePrefix covers servicePrefixOf's non-default branch:
// a configured prefix replaces the default one in the service the passphrase is
// stored under.
func TestBrokerCustomServicePrefix(t *testing.T) {
	secret := &fakeSecret{lookupFound: false}
	tty := &fakeTTY{answer: "typed-pass"}
	b := Broker{
		Secret: secret, TTY: tty, Log: &fakeLogger{},
		Config: Config{ServicePrefix: "MyPrefix"},
	}

	if _, ok := b.Answer("Enter passphrase for /home/u/.ssh/id_rsa:"); !ok {
		t.Fatal("Answer ok = false, want true")
	}
	if len(secret.stored) != 1 || secret.stored[0].service != "MyPrefix-id_rsa" {
		t.Fatalf("stored = %+v, want service MyPrefix-id_rsa", secret.stored)
	}
}
