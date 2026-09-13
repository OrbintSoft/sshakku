package keys

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/run/runtest"
)

// helperThatIsThere writes a stand-in for the askpass helper and returns its
// path. Nothing ever runs it: what is under test is whether the loader looks
// for it before it starts asking people for secrets, and a file is the whole of
// what looking can find.
func helperThatIsThere(t *testing.T, dir string) string {
	t.Helper()
	prog := filepath.Join(dir, "sshakku-askpass")
	require.NoError(t, os.WriteFile(prog, []byte("a program\n"), 0o700)) //nolint:gosec // G306: a file standing in for a program, which is not one unless it can be run
	return prog
}

// keyIn writes a private key file for the enumerator to find. Its contents are
// never parsed here — the loader reaches the program-to-ask-with question long
// before anything reads a key.
func keyIn(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "id_ed25519"), []byte("key"), 0o600))
}

// TestNoKeyIsLoadedWithNoProgramToAskWith. ssh-add takes a passphrase from one
// place only — the program named in SSH_ASKPASS — and OpenSSH answers a program
// that is not there by handing ssh-add an empty passphrase, which comes back
// indistinguishable from a wrong one. Going on from there costs the user three
// prompts answered correctly, a wallet entry written off as stale, and a key
// skipped in every later shell. So the question is asked once, before any of
// that can begin.
func TestNoKeyIsLoadedWithNoProgramToAskWith(t *testing.T) {
	dir := t.TempDir()
	keyIn(t, dir)
	r := runtest.NewRunner()
	prompter := &fakePrompter{avail: true, pass: "hunter2"}
	secret := &fakeSecret{lookupPass: "the-right-one", lookupFound: true}

	err := Loader{
		Keys:   Enumerator{Dir: dir},
		Runner: r,
		Secret: secret,
		Prompt: prompter,
		Adder:  ExecKeyAdder{AskpassProg: filepath.Join(dir, "nothing-here", "sshakku-askpass")},
	}.LoadKeys(t.Context())

	require.Error(t, err, "a session that cannot ask for a passphrase has to say so")
	assert.Contains(t, err.Error(), "sshakku-askpass",
		"the sentence has to name the program that is not there, since that is the thing to put back")
	assert.Empty(t, prompter.calls,
		"nobody may be asked for a passphrase that could not have been used")
	assert.Empty(t, r.Calls,
		"and nothing may be run, since every answer it gave would be read as a wrong passphrase")
}

// TestNothingIsAskedOfTheWalletWithNoProgramToAskWith. Without the check the
// loader gets as far as spending the stored passphrase, and the answer it
// misreads is what produces "stored passphrase is stale" — a line that sends
// somebody to their wallet to fix an entry that was always right. The wallet is
// given nothing to be blamed for: it is never opened at all.
//
// The agent and the fingerprinter answer here, so that arriving at the wallet
// is what this session would otherwise do rather than something it was stopped
// short of by an unrelated failure.
func TestNothingIsAskedOfTheWalletWithNoProgramToAskWith(t *testing.T) {
	dir := t.TempDir()
	keyIn(t, dir)
	r := runtest.NewRunner().On("ssh-add", agentEmpty()).On("ssh-keygen", keygen("SHA256:NEW"))
	secret := &fakeSecret{lookupPass: "the-right-one", lookupFound: true}
	log := &fakeLogger{}

	err := Loader{
		Keys:   Enumerator{Dir: dir},
		Runner: r,
		Secret: secret,
		Prompt: &fakePrompter{avail: true},
		Adder:  ExecKeyAdder{AskpassProg: filepath.Join(dir, "nothing-here", "sshakku-askpass")},
		Log:    log,
	}.LoadKeys(t.Context())

	require.Error(t, err)
	assert.Zero(t, secret.lookupCalls,
		"a passphrase that was never spent cannot be the reason this failed, so it is never fetched")
	assert.False(t, log.contains("is stale"),
		"and nothing in the log may send the user to their wallet over it")
}

// TestAnAccountWithNoKeysIsToldNothingAboutTheProgramToAskWith. Where there is
// no key to load there is nothing to ask about, so a missing helper is not this
// session's problem — and saying so would put an error in front of somebody
// with nothing to fix.
func TestAnAccountWithNoKeysIsToldNothingAboutTheProgramToAskWith(t *testing.T) {
	err := Loader{
		Keys:   Enumerator{Dir: filepath.Join(t.TempDir(), "no-ssh-directory-here")},
		Runner: runtest.NewRunner(),
		Adder:  ExecKeyAdder{AskpassProg: filepath.Join("nothing-here", "sshakku-askpass")},
	}.LoadKeys(t.Context())

	require.NoError(t, err)
}

// TestAProgramThatIsThereIsOneToAskWith: the check says yes to the ordinary
// case, which is every machine the install put the helper on.
func TestAProgramThatIsThereIsOneToAskWith(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, ExecKeyAdder{AskpassProg: helperThatIsThere(t, dir)}.CanAsk())
}

// TestADirectoryIsNotAProgramToAskWith. A path that resolves to something other
// than a file is found by any check that only asks whether the name exists, and
// ssh-add would meet it as the missing program it is.
func TestADirectoryIsNotAProgramToAskWith(t *testing.T) {
	dir := t.TempDir()
	standing := filepath.Join(dir, "sshakku-askpass")
	require.NoError(t, os.Mkdir(standing, 0o700))

	require.Error(t, ExecKeyAdder{AskpassProg: standing}.CanAsk())
}
