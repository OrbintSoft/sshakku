package keys

import (
	"errors"
	"testing"
)

// fakeTTY scripts one terminal answer and records how it was prompted.
type fakeTTY struct {
	answer string
	err    error
	calls  []fakeTTYCall
}

type fakeTTYCall struct {
	prompt string
	secret bool
}

func (t *fakeTTY) Prompt(prompt string, secret bool) (string, error) {
	t.calls = append(t.calls, fakeTTYCall{prompt, secret})
	return t.answer, t.err
}

func TestParsePassphrasePrompt(t *testing.T) {
	tests := []struct {
		name    string
		prompt  string
		wantKey string
		wantOK  bool
	}{
		{"ssh client quoted", "Enter passphrase for key '/home/u/.ssh/id_ed25519': ", "/home/u/.ssh/id_ed25519", true},
		{"ssh-add unquoted", "Enter passphrase for /home/u/.ssh/id_rsa: ", "/home/u/.ssh/id_rsa", true},
		{"quoted without key word", "Enter passphrase for '/home/u/.ssh/id_dsa': ", "/home/u/.ssh/id_dsa", true},
		{"login password", "user@host's password: ", "", false},
		{"host key confirmation", "Are you sure you want to continue connecting (yes/no/[fingerprint])? ", "", false},
		{"empty", "", "", false},
		{"unrelated", "Some random text", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotKey, gotOK := ParsePassphrasePrompt(tc.prompt)
			if gotOK != tc.wantOK || gotKey != tc.wantKey {
				t.Errorf("ParsePassphrasePrompt(%q) = (%q, %v), want (%q, %v)", tc.prompt, gotKey, gotOK, tc.wantKey, tc.wantOK)
			}
		})
	}
}

func TestBrokerWalletHit(t *testing.T) {
	secret := &fakeSecret{lookupPass: "stored", lookupFound: true}
	tty := &fakeTTY{}
	log := &fakeLogger{}
	b := Broker{Secret: secret, TTY: tty, Log: log}

	reply, ok := b.Answer("Enter passphrase for key '/home/u/.ssh/id_rsa': ")
	if !ok || reply != "stored" {
		t.Fatalf("Answer = (%q, %v), want (stored, true)", reply, ok)
	}
	if len(tty.calls) != 0 {
		t.Fatalf("a wallet hit must not touch the terminal, got %d prompts", len(tty.calls))
	}
	if !log.contains("from the wallet") {
		t.Fatalf("expected a wallet-hit log, got %v", log.lines)
	}
}

func TestBrokerWalletMissPromptsAndStores(t *testing.T) {
	secret := &fakeSecret{lookupFound: false}
	tty := &fakeTTY{answer: "typed"}
	b := Broker{Secret: secret, TTY: tty, Log: &fakeLogger{}}

	reply, ok := b.Answer("Enter passphrase for key '/home/u/.ssh/id_rsa': ")
	if !ok || reply != "typed" {
		t.Fatalf("Answer = (%q, %v), want (typed, true)", reply, ok)
	}
	if len(tty.calls) != 1 || !tty.calls[0].secret {
		t.Fatalf("want one no-echo passphrase prompt, got %+v", tty.calls)
	}
	if len(secret.stored) != 1 || secret.stored[0].service != "SSH-Key-id_rsa" || secret.stored[0].passphrase != "typed" {
		t.Fatalf("the typed passphrase must be stored under SSH-Key-id_rsa, got %v", secret.stored)
	}
}

func TestBrokerWalletMissExcludedByPolicyNotStored(t *testing.T) {
	secret := &fakeSecret{lookupFound: false}
	tty := &fakeTTY{answer: "typed"}
	log := &fakeLogger{}
	b := Broker{
		Secret: secret, TTY: tty, Log: log,
		Config: Config{WalletStore: func(keyname string) bool { return keyname != "id_rsa" }},
	}

	reply, ok := b.Answer("Enter passphrase for key '/home/u/.ssh/id_rsa': ")
	if !ok || reply != "typed" {
		t.Fatalf("Answer = (%q, %v), want (typed, true)", reply, ok)
	}
	if len(secret.stored) != 0 {
		t.Fatalf("an excluded key must not be stored, got %v", secret.stored)
	}
	if !log.contains("wallet-store policy excludes id_rsa") {
		t.Fatalf("expected an excluded-by-policy log, got %v", log.lines)
	}
}

func TestBrokerNonPassphrasePassThrough(t *testing.T) {
	secret := &fakeSecret{}
	tty := &fakeTTY{answer: "yes"}
	b := Broker{Secret: secret, TTY: tty, Log: &fakeLogger{}}

	reply, ok := b.Answer("Are you sure you want to continue connecting (yes/no/[fingerprint])? ")
	if !ok || reply != "yes" {
		t.Fatalf("Answer = (%q, %v), want (yes, true)", reply, ok)
	}
	if len(tty.calls) != 1 || tty.calls[0].secret {
		t.Fatalf("a confirmation must be prompted with echo on, got %+v", tty.calls)
	}
	if len(secret.stored) != 0 {
		t.Fatalf("a non-passphrase prompt must not store anything, got %v", secret.stored)
	}
}

// TestBrokerNoTerminal confirms that no controlling terminal — a normal,
// expected condition for a non-interactive invocation — declines the prompt
// without ok, and is logged at INFO, not ERROR.
func TestBrokerNoTerminal(t *testing.T) {
	secret := &fakeSecret{lookupFound: false}
	tty := &fakeTTY{err: ErrNoTerminal}
	log := &fakeLogger{}
	b := Broker{Secret: secret, TTY: tty, Log: log}

	if reply, ok := b.Answer("Enter passphrase for key '/home/u/.ssh/id_rsa': "); ok || reply != "" {
		t.Fatalf("Answer = (%q, %v), want (\"\", false) with no terminal", reply, ok)
	}
	if !log.contains("INFO askpass: no terminal") {
		t.Fatalf("expected an INFO no-terminal log, got %v", log.lines)
	}
}

// TestBrokerPromptFailureLogsError confirms that a genuine prompt failure —
// anything other than no controlling terminal — still logs at ERROR.
func TestBrokerPromptFailureLogsError(t *testing.T) {
	secret := &fakeSecret{lookupFound: false}
	tty := &fakeTTY{err: errors.New("terminal ioctl boom")}
	log := &fakeLogger{}
	b := Broker{Secret: secret, TTY: tty, Log: log}

	if reply, ok := b.Answer("Enter passphrase for key '/home/u/.ssh/id_rsa': "); ok || reply != "" {
		t.Fatalf("Answer = (%q, %v), want (\"\", false)", reply, ok)
	}
	if !log.contains("ERROR askpass: no terminal") {
		t.Fatalf("expected an ERROR log for a genuine prompt failure, got %v", log.lines)
	}
}

// TestBrokerLookupErrorLogsInfoNotError confirms a Secret.Lookup failure —
// usually the configured backend not being reachable in this environment —
// is logged at INFO and still falls through to the terminal prompt.
func TestBrokerLookupErrorLogsInfoNotError(t *testing.T) {
	secret := &fakeSecret{lookupErr: errors.New("dbus: not reachable")}
	tty := &fakeTTY{answer: "typed-pass"}
	log := &fakeLogger{}
	b := Broker{Secret: secret, TTY: tty, Log: log}

	reply, ok := b.Answer("Enter passphrase for key '/home/u/.ssh/id_rsa': ")
	if !ok || reply != "typed-pass" {
		t.Fatalf("Answer = (%q, %v), want (typed-pass, true)", reply, ok)
	}
	if !log.contains("INFO askpass: secret lookup") {
		t.Fatalf("expected an INFO secret-lookup log, got %v", log.lines)
	}
	for _, l := range log.lines {
		if len(l) >= 5 && l[:5] == "ERROR" {
			t.Fatalf("an unreachable backend must not log at ERROR, got %v", log.lines)
		}
	}
}
