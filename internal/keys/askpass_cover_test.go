package keys

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/keys/prompt"
)

// TestParsePassphrasePromptWhitespaceKeyfile covers the empty-keyfile guard:
// a prompt whose captured path is only whitespace is not treated as a real
// key-passphrase request.
func TestParsePassphrasePromptWhitespaceKeyfile(t *testing.T) {
	_, ok := ParsePassphrasePrompt("Enter passphrase for   :")
	assert.False(t, ok,
		"a prompt naming no key at all is not a key's passphrase: there is no entry it could be stored under")
}

// TestBrokerNonPassphraseNoTerminalLogsInfo covers the non-passphrase prompt
// path when there is no terminal: a normal, expected condition logged at INFO,
// answered with a declined (ok=false) reply.
func TestBrokerNonPassphraseNoTerminalLogsInfo(t *testing.T) {
	log := &fakeLogger{}
	tty := &fakeTTY{err: prompt.ErrNoTerminal}
	b := Broker{Secret: &fakeSecret{}, TTY: tty, Log: log}

	reply, ok := b.Answer(t.Context(), "Please enter your login password:")
	assert.False(t, ok, "with nowhere to ask, the question must be declined rather than answered")
	assert.Empty(t, reply, "and nothing may be handed to ssh as though it were the answer")
	assert.Truef(t, log.contains("INFO askpass: no terminal for prompt"),
		"a non-interactive invocation having no terminal is expected, not broken: %v", log.lines)
}

// TestBrokerNonPassphraseHardErrorLogsError covers the non-passphrase prompt
// path when prompting fails for an operator-actionable reason.
func TestBrokerNonPassphraseHardErrorLogsError(t *testing.T) {
	log := &fakeLogger{}
	tty := &fakeTTY{err: errors.New("terminal ioctl boom")}
	b := Broker{Secret: &fakeSecret{}, TTY: tty, Log: log}

	reply, ok := b.Answer(t.Context(), "Please enter your login password:")
	assert.False(t, ok, "a terminal that failed cannot answer")
	assert.Empty(t, reply, "and nothing may be handed to ssh as though it were the answer")
	assert.Truef(t, log.contains("ERROR askpass: no terminal for prompt"),
		"a terminal that broke is something an operator has to fix, unlike simply not having one: %v", log.lines)
}

// TestBrokerStoreErrorLogged covers storePassphrase's store-failure branch:
// after a wallet miss the user types a passphrase, but persisting it fails —
// logged at ERROR while the typed value is still returned to ssh.
func TestBrokerStoreErrorLogged(t *testing.T) {
	log := &fakeLogger{}
	secret := &fakeSecret{lookupFound: false, storeErr: errors.New("wallet write denied")}
	tty := &fakeTTY{answer: "typed-pass"}
	b := Broker{Secret: secret, TTY: tty, Log: log}

	reply, ok := b.Answer(t.Context(), "Enter passphrase for /home/u/.ssh/id_rsa:")
	require.True(t, ok, "a wallet that would not take the passphrase must not cost the user their key")
	assert.Equal(t, "typed-pass", reply, "what they typed still opens it")
	assert.Truef(t, log.contains("ERROR askpass: store passphrase for id_rsa"),
		"but the log must say it was not saved, or they are asked again next time with no explanation: %v", log.lines)
}

// TestBrokerNilLoggerDoesNotPanic covers logf's nil-Logger guard: a Broker with
// no Logger answers normally without logging.
func TestBrokerNilLoggerDoesNotPanic(t *testing.T) {
	secret := &fakeSecret{lookupPass: "stored", lookupFound: true}
	b := Broker{Secret: secret, TTY: &fakeTTY{}, Log: nil}

	reply, ok := b.Answer(t.Context(), "Enter passphrase for /home/u/.ssh/id_rsa:")
	require.True(t, ok, "keeping no session log is a choice, and it must not cost the user their key")
	assert.Equal(t, "stored", reply, "the passphrase in the wallet still answers")
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

	_, ok := b.Answer(t.Context(), "Enter passphrase for /home/u/.ssh/id_rsa:")
	require.True(t, ok, "the key must open")
	require.Lenf(t, secret.stored, 1, "and the passphrase must be saved once: %+v", secret.stored)
	assert.Equal(t, "MyPrefix-id_rsa", secret.stored[0].service,
		"under the name this configuration writes, or a later lookup goes looking somewhere else")
}
