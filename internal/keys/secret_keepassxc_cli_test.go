package keys

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingRunner records every command it was asked to run and answers from a
// scripted list. It replaces the process boundary — upstream of every decision
// the backend makes — and records what crossed it, so a test can assert on the
// argv and the standard input rather than on what the backend meant to send.
type recordingRunner struct {
	results []Result
	errs    []error
	calls   []Cmd
}

func (r *recordingRunner) Run(c Cmd) (Result, error) {
	r.calls = append(r.calls, c)
	i := len(r.calls) - 1
	if i < len(r.errs) && r.errs[i] != nil {
		return Result{}, r.errs[i]
	}
	if i < len(r.results) {
		return r.results[i], nil
	}
	return Result{}, nil
}

// countingPrompter answers with a set password and counts how often it was asked.
type countingPrompter struct {
	password string
	err      error
	asked    int
}

func (p *countingPrompter) Prompt(string) (string, error) {
	p.asked++
	return p.password, p.err
}

func (p *countingPrompter) Available() bool { return true }

// cliBackend wires a CLI backend to the fakes above.
func cliBackend(runner *recordingRunner, prompter *countingPrompter) *KeePassXCCLIBackend {
	return &KeePassXCCLIBackend{
		Runner:   runner,
		Prompter: prompter,
		Database: "/db.kdbx",
	}
}

// TestKeePassXCCLIPasswordNeverReachesArgv is the assertion the whole stdin
// arrangement exists for: `ps` must not show the database password. It reads
// the recorded argv, not the code's intent.
func TestKeePassXCCLIPasswordNeverReachesArgv(t *testing.T) {
	const password = "the-database-password"
	runner := &recordingRunner{results: []Result{{Code: 0, Stdout: []byte("passphrase\n")}}}
	b := cliBackend(runner, &countingPrompter{password: password})

	_, _, err := b.Lookup(defaultServicePrefix + "-id_ed25519")
	require.NoError(t, err, "reading an entry out of the database must succeed")
	require.Len(t, runner.calls, 1, "one lookup is one command")
	call := runner.calls[0]
	for _, arg := range call.Args {
		assert.NotContains(t, arg, password,
			"argv is world-readable on this machine: a database password there is readable by every other user")
	}
	assert.Contains(t, call.Stdin, password,
		"the database password must be fed on standard input, or the command cannot open the database")
}

// TestKeePassXCCLILookupReturnsWhatWasStored is what the whole route exists to
// do, and what every other case here takes for granted while asserting around
// it: the passphrase handed back is the one keepassxc-cli printed, with the
// newline the command appends to its output removed. A passphrase carrying that
// newline is not the passphrase, and the key it was saved for stops opening.
func TestKeePassXCCLILookupReturnsWhatWasStored(t *testing.T) {
	const passphrase = "the-stored-passphrase"
	runner := &recordingRunner{results: []Result{{Code: 0, Stdout: []byte(passphrase + "\n")}}}
	b := cliBackend(runner, &countingPrompter{password: "p"})

	got, found, err := b.Lookup(defaultServicePrefix + "-id_ed25519")
	require.NoError(t, err, "reading a stored passphrase must succeed")
	assert.True(t, found, "the entry is in the database, so it must be reported found")
	assert.Equal(t, passphrase, got,
		"the passphrase must be exactly what was stored: keepassxc-cli's trailing newline is not part of it")
}

// TestKeePassXCCLIStoreKeepsThePassphraseOffArgvToo covers the second secret
// the CLI needs: the key's own passphrase, which follows the database password
// on standard input.
func TestKeePassXCCLIStoreKeepsThePassphraseOffArgvToo(t *testing.T) {
	const dbPassword = "db-password"
	const passphrase = "the-key-passphrase"
	runner := &recordingRunner{results: []Result{
		{Code: 1}, // the existence check: no such entry yet
		{Code: 0}, // creating the group
		{Code: 0}, // the add
	}}
	b := cliBackend(runner, &countingPrompter{password: dbPassword})

	require.NoError(t, b.Store(defaultServicePrefix+"-id_ed25519", "", passphrase), "saving a passphrase must succeed")
	require.Len(t, runner.calls, 3, "a key with no entry yet is looked up, then its group made, then written")
	write := runner.calls[2]
	for _, arg := range write.Args {
		assert.NotContains(t, arg, passphrase,
			"argv is world-readable on this machine: a passphrase there is readable by every other user")
		assert.NotContains(t, arg, dbPassword, "and so would a database password be")
	}
	assert.Equal(t, dbPassword+"\n"+passphrase+"\n", write.Stdin,
		"the database password comes first and the passphrase second, in the order keepassxc-cli asks for them")
}

func TestKeePassXCCLIStoreAddsThenEdits(t *testing.T) {
	t.Run("a key with no entry yet is added, into a group that is made first", func(t *testing.T) {
		runner := &recordingRunner{results: []Result{{Code: 1}, {Code: 0}, {Code: 0}}}
		b := cliBackend(runner, &countingPrompter{password: "p"})
		require.NoError(t, b.Store(defaultServicePrefix+"-new", "", "x"), "saving a passphrase must succeed")
		require.Len(t, runner.calls, 3, "a key with no entry yet is looked up, then its group made, then written")
		// A real keepassxc-cli refuses to add an entry to a group that does
		// not exist, and a fresh database has none.
		assert.Equal(t, "mkdir", runner.calls[1].Args[0], "the group must be made before anything can be put in it")
		assert.Equal(t, "add", runner.calls[2].Args[0], "and a key with no entry yet is added")
	})

	t.Run("a key already stored is edited in place", func(t *testing.T) {
		runner := &recordingRunner{results: []Result{
			{Code: 0, Stdout: []byte("old\n")},
			{Code: 0},
		}}
		b := cliBackend(runner, &countingPrompter{password: "p"})
		require.NoError(t, b.Store(defaultServicePrefix+"-existing", "", "x"), "replacing a passphrase must succeed")
		// The group is already there, so an edit goes straight to the write.
		require.Len(t, runner.calls, 2, "a key that is already stored is looked up, then written")
		assert.Equal(t, "edit", runner.calls[1].Args[0],
			"an entry that is there must be edited; adding again leaves a second copy of the secret in the database")
	})
}

// TestKeePassXCCLIAsksForThePasswordOnlyOnce keeps the route from turning one
// shell into one prompt per key.
func TestKeePassXCCLIAsksForThePasswordOnlyOnce(t *testing.T) {
	runner := &recordingRunner{results: []Result{
		{Code: 0, Stdout: []byte("a\n")},
		{Code: 0, Stdout: []byte("b\n")},
	}}
	prompter := &countingPrompter{password: "p"}
	b := cliBackend(runner, prompter)

	_, _, err := b.Lookup(defaultServicePrefix + "-one")
	require.NoError(t, err, "the first lookup must succeed")
	_, _, err = b.Lookup(defaultServicePrefix + "-two")
	require.NoError(t, err, "and so must the second")
	assert.Equal(t, 1, prompter.asked,
		"the database password is asked for once for the whole process, not once per key")
}

func TestKeePassXCCLILookupOfAnAbsentEntryIsAMiss(t *testing.T) {
	runner := &recordingRunner{results: []Result{{Code: 1, Stderr: []byte("Could not find entry\n")}}}
	b := cliBackend(runner, &countingPrompter{password: "p"})

	_, found, err := b.Lookup(defaultServicePrefix + "-absent")
	require.NoError(t, err, "a passphrase that was never stored is not an error")
	assert.False(t, found, "and nothing may be reported found")
}

// TestKeePassXCCLIReportsARefusedPassword covers the case this route was
// written defensively for: keepassxc-cli has no documented way to take the
// password on standard input, so if it ever stops doing so, that has to be
// said rather than surface as an unexplained miss.
func TestKeePassXCCLIReportsARefusedPassword(t *testing.T) {
	runner := &recordingRunner{results: []Result{{
		Code:   1,
		Stderr: []byte("Failed to open the terminal to read the password\n"),
	}}}
	b := cliBackend(runner, &countingPrompter{password: "p"})

	_, _, err := b.Lookup(defaultServicePrefix + "-id_ed25519")
	assert.ErrorIs(t, err, ErrPasswordNotAccepted,
		"a database that would not take the password must be said so: a refused interface is not an empty wallet")
}

// TestKeePassXCCLICanDelete is the difference from the local-protocol route,
// which has no verb for it: here `sshakku forget` really removes the entry.
func TestKeePassXCCLICanDelete(t *testing.T) {
	runner := &recordingRunner{results: []Result{{Code: 0}}}
	b := cliBackend(runner, &countingPrompter{password: "p"})

	require.NoError(t, b.Delete(defaultServicePrefix+"-id_ed25519"), "forgetting a passphrase must succeed")
	require.NotEmpty(t, runner.calls, "the database must actually be asked")
	assert.Equal(t, "rm", runner.calls[0].Args[0], "and asked to remove the entry")
	assert.Contains(t, strings.Join(runner.calls[0].Args, " "), "SSHakku/"+defaultServicePrefix+"-id_ed25519",
		"exactly the entry inside SSHakku's own group, not one of the user's own elsewhere in the database")
}

func TestKeePassXCCLIDeleteOfAnAbsentEntryIsSuccess(t *testing.T) {
	runner := &recordingRunner{results: []Result{{Code: 1, Stderr: []byte("Entry not found\n")}}}
	b := cliBackend(runner, &countingPrompter{password: "p"})

	require.NoError(t, b.Delete(defaultServicePrefix+"-gone"),
		"a passphrase that is already not there is the outcome that was asked for")
}

func TestKeePassXCCLIListsWhatItStored(t *testing.T) {
	runner := &recordingRunner{results: []Result{{
		Code:   0,
		Stdout: []byte(defaultServicePrefix + "-id_ed25519\n" + defaultServicePrefix + "-id_rsa\nnested/\n\n"),
	}}}
	b := cliBackend(runner, &countingPrompter{password: "p"})

	got, err := b.List()
	require.NoError(t, err, "listing what the database holds must succeed")
	assert.Equal(t, []string{defaultServicePrefix + "-id_ed25519", defaultServicePrefix + "-id_rsa"}, got,
		"every entry must be named, and a trailing slash marks a group rather than an entry")
}

func TestKeePassXCCLIWithNoDatabaseSaysSo(t *testing.T) {
	b := &KeePassXCCLIBackend{Runner: &recordingRunner{}, Prompter: &countingPrompter{password: "p"}}
	_, _, err := b.Lookup(defaultServicePrefix + "-id_ed25519")
	assert.ErrorIs(t, err, ErrNoDatabase,
		"a route with no database configured must say which piece is missing, not fail as an empty wallet")
}

func TestKeePassXCCLIPassesTheKeyFileWhenConfigured(t *testing.T) {
	runner := &recordingRunner{results: []Result{{Code: 0, Stdout: []byte("x\n")}}}
	b := cliBackend(runner, &countingPrompter{password: "p"})
	b.KeyFile = "/db.key"

	_, _, err := b.Lookup(defaultServicePrefix + "-id_ed25519")
	require.NoError(t, err, "reading an entry out of a key-file-protected database must succeed")
	require.NotEmpty(t, runner.calls, "the database must actually be asked")
	assert.Contains(t, strings.Join(runner.calls[0].Args, " "), "--key-file /db.key",
		"a database that also needs a key file cannot be opened without one")
}

func TestKeePassXCCLIReportsARefusedPrompt(t *testing.T) {
	refused := errors.New("the user dismissed the dialog")
	b := cliBackend(&recordingRunner{}, &countingPrompter{err: refused})

	_, _, err := b.Lookup(defaultServicePrefix + "-id_ed25519")
	assert.ErrorIs(t, err, refused, "a user who dismissed the prompt has answered, and must be obeyed")
}

// runnerErr is a runner that cannot start the command at all — keepassxc-cli
// not installed, for instance, which is a different thing from it failing.
func runnerErr(n int) *recordingRunner {
	r := &recordingRunner{}
	for i := 0; i < n; i++ {
		r.errs = append(r.errs, errors.New("keepassxc-cli: no such file"))
	}
	return r
}

func TestKeePassXCCLIReportsACommandThatCannotRun(t *testing.T) {
	for _, tc := range []struct {
		name string
		call func(*KeePassXCCLIBackend) error
	}{
		{"lookup", func(b *KeePassXCCLIBackend) error { _, _, err := b.Lookup(defaultServicePrefix + "-k"); return err }},
		{"store", func(b *KeePassXCCLIBackend) error { return b.Store(defaultServicePrefix+"-k", "", "p") }},
		{"delete", func(b *KeePassXCCLIBackend) error { return b.Delete(defaultServicePrefix + "-k") }},
		{"list", func(b *KeePassXCCLIBackend) error { _, err := b.List(); return err }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := cliBackend(runnerErr(3), &countingPrompter{password: "p"})
			assert.Error(t, tc.call(b), "a command that could not run must be reported, not read as an empty wallet")
		})
	}
}

func TestKeePassXCCLIStoreReportsAFailureItCannotName(t *testing.T) {
	runner := &recordingRunner{results: []Result{
		{Code: 1},
		{Code: 0},
		{Code: 1, Stderr: []byte("Group SSHakku not found\nsecond line\n")},
	}}
	b := cliBackend(runner, &countingPrompter{password: "p"})

	err := b.Store(defaultServicePrefix+"-k", "", "x")
	require.Error(t, err, "a write that failed must be reported, not passed over")
	assert.Contains(t, err.Error(), "Group SSHakku not found",
		"and it must carry KeePassXC's own words, which are the only account of what went wrong")
	assert.NotContains(t, err.Error(), "second line",
		"one line of it, so the reason fits where a user reads it")
}

func TestKeePassXCCLIReportsARefusedPasswordOnEveryOperation(t *testing.T) {
	refusal := Result{Code: 1, Stderr: []byte("Failed to open the terminal for the password\n")}

	t.Run("store", func(t *testing.T) {
		// The existence check misses cleanly and the group is made; the write
		// is what is refused.
		runner := &recordingRunner{results: []Result{{Code: 1}, {Code: 0}, refusal}}
		b := cliBackend(runner, &countingPrompter{password: "p"})
		assert.ErrorIs(t, b.Store(defaultServicePrefix+"-k", "", "x"), ErrPasswordNotAccepted,
			"a database that would not take the password must be said so")
	})

	t.Run("delete", func(t *testing.T) {
		b := cliBackend(&recordingRunner{results: []Result{refusal}}, &countingPrompter{password: "p"})
		assert.ErrorIs(t, b.Delete(defaultServicePrefix+"-k"), ErrPasswordNotAccepted,
			"a database that would not take the password must be said so, or forget claims to have removed nothing")
	})

	t.Run("list", func(t *testing.T) {
		b := cliBackend(&recordingRunner{results: []Result{refusal}}, &countingPrompter{password: "p"})
		_, err := b.List()
		assert.ErrorIs(t, err, ErrPasswordNotAccepted,
			"a database that would not take the password must be said so, not listed as empty")
	})
}

func TestKeePassXCCLIListOfAnAbsentGroupIsEmpty(t *testing.T) {
	runner := &recordingRunner{results: []Result{{Code: 1, Stderr: []byte("Cannot find group SSHakku\n")}}}
	b := cliBackend(runner, &countingPrompter{password: "p"})

	got, err := b.List()
	require.NoError(t, err, "a database SSHakku has never written to is not an error")
	assert.Empty(t, got, "and nothing may be listed")
}

func TestFirstLine(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"one line", "just this", "just this"},
		{"several lines keeps the first", "first\nsecond\nthird", "first"},
		{"surrounding blank space is dropped", "  \n padded \n more \n", "padded"},
		{"nothing", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equalf(t, tc.want, firstLine([]byte(tc.in)), "firstLine(%q)", tc.in)
		})
	}
}

// TestKeePassXCCLIStoreReportsAWriteThatCannotRun covers the case where the
// existence check works and the write itself cannot start.
func TestKeePassXCCLIStoreReportsAWriteThatCannotRun(t *testing.T) {
	runner := &recordingRunner{
		results: []Result{{Code: 1}, {Code: 0}},
		errs:    []error{nil, nil, errors.New("keepassxc-cli vanished")},
	}
	b := cliBackend(runner, &countingPrompter{password: "p"})

	assert.Error(t, b.Store(defaultServicePrefix+"-k", "", "x"),
		"a write that could not start must be reported, not read as saved")
}

// TestKeePassXCCLIStoreReportsAGroupThatCannotBeMade covers the step a real
// keepassxc-cli made necessary: without the group, the add cannot land.
func TestKeePassXCCLIStoreReportsAGroupThatCannotBeMade(t *testing.T) {
	runner := &recordingRunner{
		results: []Result{{Code: 1}},
		errs:    []error{nil, errors.New("keepassxc-cli vanished")},
	}
	b := cliBackend(runner, &countingPrompter{password: "p"})

	assert.Error(t, b.Store(defaultServicePrefix+"-k", "", "x"),
		"a group that could not be made must be reported: nothing can be written into it")
}
