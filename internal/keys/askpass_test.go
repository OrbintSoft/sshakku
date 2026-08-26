package keys

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/keys/prompt"
	"github.com/OrbintSoft/sshakku/internal/keys/wallet"
)

// errDbusNotReachable is the failure this test hands its seam, standing for a real one the
// code under test cannot be made to produce on demand.
var errDbusNotReachable = errors.New("dbus: not reachable")

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
			assert.Equalf(t, tc.wantOK, gotOK,
				"whether %q is a key's passphrase decides whether the answer goes into the wallet", tc.prompt)
			assert.Equalf(t, tc.wantKey, gotKey, "and which key it is decides which entry it goes under: %q", tc.prompt)
		})
	}
}

func TestBrokerWalletHit(t *testing.T) {
	secret := &fakeSecret{lookupPass: "stored", lookupFound: true}
	tty := &fakeTTY{}
	log := &fakeLogger{}
	b := Broker{Secret: secret, TTY: tty, Log: log}

	reply, ok := b.Answer(t.Context(), "Enter passphrase for key '/home/u/.ssh/id_rsa': ")
	require.True(t, ok, "a passphrase in the wallet must be answered with")
	assert.Equal(t, "stored", reply, "and it must be the one that was stored")
	assert.Emptyf(t, tty.calls, "the whole point is that the user is not asked: %+v", tty.calls)
	assert.Truef(t, log.contains("from the wallet"), "and the log must say where the answer came from: %v", log.lines)
}

func TestBrokerWalletMissPromptsAndStores(t *testing.T) {
	secret := &fakeSecret{lookupFound: false}
	tty := &fakeTTY{answer: "typed"}
	b := Broker{Secret: secret, TTY: tty, Log: &fakeLogger{}}

	reply, ok := b.Answer(t.Context(), "Enter passphrase for key '/home/u/.ssh/id_rsa': ")
	require.True(t, ok, "a passphrase the user typed must be answered with")
	assert.Equal(t, "typed", reply, "and it must be what they typed")
	require.Lenf(t, tty.calls, 1, "they are asked once: %+v", tty.calls)
	assert.True(t, tty.calls[0].secret, "with the echo off, or the passphrase is on screen for anyone behind them")
	require.Lenf(t, secret.stored, 1, "and it must be saved once: %v", secret.stored)
	assert.Equal(t, wallet.DefaultServicePrefix+"-id_rsa", secret.stored[0].service,
		"under the name a later lookup goes by")
	assert.Equal(t, "typed", secret.stored[0].passphrase, "so the next time nobody is asked at all")
}

func TestBrokerWalletMissExcludedByPolicyNotStored(t *testing.T) {
	secret := &fakeSecret{lookupFound: false}
	tty := &fakeTTY{answer: "typed"}
	log := &fakeLogger{}
	b := Broker{
		Secret: secret, TTY: tty, Log: log,
		Config: Config{WalletStore: func(keyname string) bool { return keyname != "id_rsa" }},
	}

	reply, ok := b.Answer(t.Context(), "Enter passphrase for key '/home/u/.ssh/id_rsa': ")
	require.True(t, ok, "the key must still open")
	assert.Equal(t, "typed", reply, "with what the user typed")
	assert.Emptyf(t, secret.stored,
		"but a key the user excluded from their wallet must not end up in it: %v", secret.stored)
	assert.Truef(t, log.contains("wallet-store policy excludes id_rsa"),
		"and the log must say why, or the setting looks ignored: %v", log.lines)
}

func TestBrokerNonPassphrasePassThrough(t *testing.T) {
	secret := &fakeSecret{}
	tty := &fakeTTY{answer: "yes"}
	b := Broker{Secret: secret, TTY: tty, Log: &fakeLogger{}}

	reply, ok := b.Answer(t.Context(), "Are you sure you want to continue connecting (yes/no/[fingerprint])? ")
	require.True(t, ok, "a question ssh asks must still reach the user")
	assert.Equal(t, "yes", reply, "and their answer must reach ssh")
	require.Lenf(t, tty.calls, 1, "they are asked once: %+v", tty.calls)
	assert.False(t, tty.calls[0].secret,
		"with the echo on: a host-key confirmation is not a secret, and hiding it makes it unanswerable")
	assert.Emptyf(t, secret.stored, "and nothing here belongs in a wallet: %v", secret.stored)
}

// TestBrokerAPasswordIsNotEchoed covers the prompts the broker passes straight
// through that are still secrets: ssh asks for a login password the same way it
// asks for a host-key confirmation, and only the confirmation is safe to show.
// Echoing a password puts it on the screen for anyone standing behind the user
// — and into the scrollback of whatever terminal they happen to be in.
func TestBrokerAPasswordIsNotEchoed(t *testing.T) {
	for _, prompt := range []string{
		"user@host's password: ",
		"Please enter your login password:",
	} {
		t.Run(prompt, func(t *testing.T) {
			tty := &fakeTTY{answer: "the-password"}
			b := Broker{Secret: &fakeSecret{}, TTY: tty, Log: &fakeLogger{}}

			reply, ok := b.Answer(t.Context(), prompt)
			require.True(t, ok, "a question ssh asks must still reach the user")
			assert.Equal(t, "the-password", reply, "and their answer must reach ssh")
			require.Lenf(t, tty.calls, 1, "they are asked once: %+v", tty.calls)
			assert.True(t, tty.calls[0].secret,
				"with the echo off: this is a secret, whatever else the broker does with it")
		})
	}
}

// TestBrokerABlankWalletEntryIsNoPassphrase is the reactive half of what the
// loader promises about a wallet entry holding nothing but blank space: it is
// not a passphrase. Handed to ssh it opens no key, and ssh does not ask again —
// so the user watches the connection fail with no prompt and nothing to type
// into. Falling through to the terminal is what the entry being blank means.
func TestBrokerABlankWalletEntryIsNoPassphrase(t *testing.T) {
	secret := &fakeSecret{lookupPass: "   ", lookupFound: true}
	tty := &fakeTTY{answer: "typed"}
	b := Broker{Secret: secret, TTY: tty, Log: &fakeLogger{}}

	reply, ok := b.Answer(t.Context(), "Enter passphrase for key '/home/u/.ssh/id_rsa': ")
	require.True(t, ok, "the key must still open")
	assert.NotEmpty(t, strings.TrimSpace(reply),
		"blank space is not a passphrase, and handing it on fails the connection with nothing asked")
	assert.Equal(t, "typed", reply, "the user is asked instead, and what they type is the answer")
	require.Lenf(t, tty.calls, 1, "so they must be asked: %+v", tty.calls)
	assert.True(t, tty.calls[0].secret, "with the echo off")
}

// TestBrokerNoTerminal confirms that no controlling terminal — a normal,
// expected condition for a non-interactive invocation — declines the prompt
// without ok, and is logged at INFO, not ERROR.
func TestBrokerNoTerminal(t *testing.T) {
	secret := &fakeSecret{lookupFound: false}
	tty := &fakeTTY{err: prompt.ErrNoTerminal}
	log := &fakeLogger{}
	b := Broker{Secret: secret, TTY: tty, Log: log}

	reply, ok := b.Answer(t.Context(), "Enter passphrase for key '/home/u/.ssh/id_rsa': ")
	assert.False(t, ok, "with nowhere to ask, the question must be declined rather than answered")
	assert.Empty(t, reply, "and nothing may be handed to ssh as though it were a passphrase")
	assert.Truef(t, log.contains("INFO askpass: no terminal"),
		"a non-interactive invocation having no terminal is expected, not broken: %v", log.lines)
}

// TestBrokerPromptFailureLogsError confirms that a genuine prompt failure —
// anything other than no controlling terminal — still logs at ERROR.
func TestBrokerPromptFailureLogsError(t *testing.T) {
	secret := &fakeSecret{lookupFound: false}
	tty := &fakeTTY{err: errTerminalIoctlBoom}
	log := &fakeLogger{}
	b := Broker{Secret: secret, TTY: tty, Log: log}

	reply, ok := b.Answer(t.Context(), "Enter passphrase for key '/home/u/.ssh/id_rsa': ")
	assert.False(t, ok, "a terminal that failed cannot answer")
	assert.Empty(t, reply, "and nothing may be handed to ssh as though it were a passphrase")
	assert.Truef(t, log.contains("ERROR askpass: no terminal"),
		"a terminal that broke is something an operator has to fix, unlike simply not having one: %v", log.lines)
}

// TestBrokerLookupErrorLogsInfoNotError confirms a Secret.Lookup failure —
// usually the configured backend not being reachable in this environment —
// is logged at INFO and still falls through to the terminal prompt.
func TestBrokerLookupErrorLogsInfoNotError(t *testing.T) {
	secret := &fakeSecret{lookupErr: errDbusNotReachable}
	tty := &fakeTTY{answer: "typed-pass"}
	log := &fakeLogger{}
	b := Broker{Secret: secret, TTY: tty, Log: log}

	reply, ok := b.Answer(t.Context(), "Enter passphrase for key '/home/u/.ssh/id_rsa': ")
	require.True(t, ok, "a wallet nobody could reach must not cost the user their key")
	assert.Equal(t, "typed-pass", reply, "they are asked instead, and the key opens")
	assert.Truef(t, log.contains("INFO askpass: secret lookup"),
		"the log must say the wallet was not reached: %v", log.lines)
	assertNothingWentWrong(t, log,
		"a machine with no wallet in this session is not a machine with something wrong with it")
}
