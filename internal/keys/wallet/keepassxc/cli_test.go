package keepassxc

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/keys/wallet"
	"github.com/OrbintSoft/sshakku/internal/run"
	"github.com/OrbintSoft/sshakku/internal/run/runtest"
)

// The failures these tests hand their seams. Each stands for a real one the
// code under test cannot be made to produce on demand.
var (
	errKeepassxcCliNoSuchFile    = errors.New("keepassxc-cli: no such file")
	errKeepassxcCliVanished      = errors.New("keepassxc-cli vanished")
	errTheUserDismissedTheDialog = errors.New("the user dismissed the dialog")
)

// countingPrompter answers with a set password and counts how often it was asked.
type countingPrompter struct {
	password string
	err      error
	asked    int
}

func (p *countingPrompter) Prompt(context.Context, string) (string, error) {
	p.asked++
	return p.password, p.err
}

func (p *countingPrompter) Available(context.Context) bool { return true }

// cliBackend wires a CLI backend to the fakes above.
func cliBackend(runner *runtest.Recorder, prompter *countingPrompter) *CLI {
	return &CLI{
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
	runner := &runtest.Recorder{Results: []run.Result{{Code: 0, Stdout: []byte("passphrase\n")}}}
	b := cliBackend(runner, &countingPrompter{password: password})

	_, _, err := b.Lookup(t.Context(), wallet.DefaultServicePrefix+"-id_ed25519")
	require.NoError(t, err, "reading an entry out of the database must succeed")
	require.Len(t, runner.Calls, 1, "one lookup is one command")
	call := runner.Calls[0]
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
	runner := &runtest.Recorder{Results: []run.Result{{Code: 0, Stdout: []byte(passphrase + "\n")}}}
	b := cliBackend(runner, &countingPrompter{password: "p"})

	got, found, err := b.Lookup(t.Context(), wallet.DefaultServicePrefix+"-id_ed25519")
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
	runner := &runtest.Recorder{Results: []run.Result{
		{Code: 1}, // the existence check: no such entry yet.
		{Code: 0}, // creating the group.
		{Code: 0}, // the add.
	}}
	b := cliBackend(runner, &countingPrompter{password: dbPassword})

	require.NoError(t, b.Store(t.Context(), wallet.DefaultServicePrefix+"-id_ed25519", "", passphrase), "saving a passphrase must succeed")
	require.Len(t, runner.Calls, 3, "a key with no entry yet is looked up, then its group made, then written")
	write := runner.Calls[2]
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
		runner := &runtest.Recorder{Results: []run.Result{{Code: 1}, {Code: 0}, {Code: 0}}}
		b := cliBackend(runner, &countingPrompter{password: "p"})
		require.NoError(t, b.Store(t.Context(), wallet.DefaultServicePrefix+"-new", "", "x"), "saving a passphrase must succeed")
		require.Len(t, runner.Calls, 3, "a key with no entry yet is looked up, then its group made, then written")
		// A real keepassxc-cli refuses to add an entry to a group that does
		// not exist, and a fresh database has none.
		assert.Equal(t, "mkdir", runner.Calls[1].Args[0], "the group must be made before anything can be put in it")
		assert.Equal(t, "add", runner.Calls[2].Args[0], "and a key with no entry yet is added")
	})

	t.Run("a key already stored is edited in place", func(t *testing.T) {
		runner := &runtest.Recorder{Results: []run.Result{
			{Code: 0, Stdout: []byte("old\n")},
			{Code: 0},
		}}
		b := cliBackend(runner, &countingPrompter{password: "p"})
		require.NoError(t, b.Store(t.Context(), wallet.DefaultServicePrefix+"-existing", "", "x"), "replacing a passphrase must succeed")
		// The group is already there, so an edit goes straight to the write.
		require.Len(t, runner.Calls, 2, "a key that is already stored is looked up, then written")
		assert.Equal(t, "edit", runner.Calls[1].Args[0],
			"an entry that is there must be edited; adding again leaves a second copy of the secret in the database")
	})
}

// TestKeePassXCCLIAsksForThePasswordOnlyOnce keeps the route from turning one
// shell into one prompt per key.
func TestKeePassXCCLIAsksForThePasswordOnlyOnce(t *testing.T) {
	runner := &runtest.Recorder{Results: []run.Result{
		{Code: 0, Stdout: []byte("a\n")},
		{Code: 0, Stdout: []byte("b\n")},
	}}
	prompter := &countingPrompter{password: "p"}
	b := cliBackend(runner, prompter)

	_, _, err := b.Lookup(t.Context(), wallet.DefaultServicePrefix+"-one")
	require.NoError(t, err, "the first lookup must succeed")
	_, _, err = b.Lookup(t.Context(), wallet.DefaultServicePrefix+"-two")
	require.NoError(t, err, "and so must the second")
	assert.Equal(t, 1, prompter.asked,
		"the database password is asked for once for the whole process, not once per key")
}

func TestKeePassXCCLILookupOfAnAbsentEntryIsAMiss(t *testing.T) {
	runner := &runtest.Recorder{Results: []run.Result{{Code: 1, Stderr: []byte("Could not find entry\n")}}}
	b := cliBackend(runner, &countingPrompter{password: "p"})

	_, found, err := b.Lookup(t.Context(), wallet.DefaultServicePrefix+"-absent")
	require.NoError(t, err, "a passphrase that was never stored is not an error")
	assert.False(t, found, "and nothing may be reported found")
}

// TestKeePassXCCLIReportsARefusedPassword covers the case this route was
// written defensively for: keepassxc-cli has no documented way to take the
// password on standard input, so if it ever stops doing so, that has to be
// said rather than surface as an unexplained miss.
func TestKeePassXCCLIReportsARefusedPassword(t *testing.T) {
	runner := &runtest.Recorder{Results: []run.Result{{
		Code:   1,
		Stderr: []byte("Failed to open the terminal to read the password\n"),
	}}}
	b := cliBackend(runner, &countingPrompter{password: "p"})

	_, _, err := b.Lookup(t.Context(), wallet.DefaultServicePrefix+"-id_ed25519")
	assert.ErrorIs(t, err, ErrPasswordNotAccepted,
		"a database that would not take the password must be said so: a refused interface is not an empty wallet")
}

// TestKeePassXCCLICanDelete is the difference from the local-protocol route,
// which has no verb for it: here `sshakku forget` really removes the entry.
//
// The removal is the second call, not the first: what precedes it is the
// database being asked where this name already is, so that what the removal
// then moves can be told apart from what was somewhere else all along.
func TestKeePassXCCLICanDelete(t *testing.T) {
	runner := &runtest.Recorder{Results: []run.Result{
		{Code: 0, Stdout: []byte("/SSHakku/" + wallet.DefaultServicePrefix + "-id_ed25519\n")},
		{Code: 0},
		{Code: 1}, // nothing found afterwards: this database has no recycle bin.
	}}
	b := cliBackend(runner, &countingPrompter{password: "p"})

	require.NoError(t, b.Delete(t.Context(), wallet.DefaultServicePrefix+"-id_ed25519"), "forgetting a passphrase must succeed")
	require.Len(t, runner.Calls, 3, "the database must actually be asked")
	assert.Equal(t, "rm", runner.Calls[1].Args[0], "and asked to remove the entry")
	assert.Contains(t, strings.Join(runner.Calls[1].Args, " "), "SSHakku/"+wallet.DefaultServicePrefix+"-id_ed25519",
		"exactly the entry inside SSHakku's own group, not one of the user's own elsewhere in the database")
}

// TestKeePassXCCLIDeleteTakesTheCopyTheRecycleBinKept is F9's own sentence:
// SSHakku "never reports a passphrase as forgotten while it is still stored".
// keepassxc-cli `rm` does not delete an entry, it moves it to the database's
// recycle bin, where the password is still there for anyone who opens the file.
//
// Where the entry went is asked of the database and never assumed: the bin is
// named in whatever language the database was made in, so no constant in this
// code could name it. The move is identified as the path that was not there
// before, which is also what keeps it to entries SSHakku itself moved.
func TestKeePassXCCLIDeleteTakesTheCopyTheRecycleBinKept(t *testing.T) {
	const service = wallet.DefaultServicePrefix + "-id_ed25519"
	// A German database, to make the point that the name is not ours to know.
	const moved = "/Papierkorb/" + service
	runner := &runtest.Recorder{Results: []run.Result{
		{Code: 0, Stdout: []byte("/SSHakku/" + service + "\n")}, // where it is now.
		{Code: 0},                               // the removal, which moves it.
		{Code: 0, Stdout: []byte(moved + "\n")}, // where it went.
		{Code: 0},                               // and the removal that deletes it.
	}}
	b := cliBackend(runner, &countingPrompter{password: "p"})

	require.NoError(t, b.Delete(t.Context(), service), "forgetting a passphrase must succeed")

	require.Len(t, runner.Calls, 4, "the entry has to be found again after the move, and removed again")
	last := runner.Calls[len(runner.Calls)-1]
	assert.Equal(t, "rm", last.Args[0], "what follows the move is a removal")
	assert.Contains(t, last.Args, moved,
		"the second removal must name where the database said the entry went")
}

// TestKeePassXCCLIDeleteLeavesASameNamedEntryItDidNotMove holds the rule above
// to F27: SSHakku only ever touches entries it put there itself. A user may
// perfectly well keep an entry of the same name somewhere else in their own
// database, and that one was not moved by this deletion — which is exactly what
// comparing against what was there before can tell, and what a search
// afterwards on its own could not.
func TestKeePassXCCLIDeleteLeavesASameNamedEntryItDidNotMove(t *testing.T) {
	const service = wallet.DefaultServicePrefix + "-id_ed25519"
	const theirs = "/Work/" + service
	const moved = "/Kosár/" + service
	runner := &runtest.Recorder{Results: []run.Result{
		{Code: 0, Stdout: []byte("/SSHakku/" + service + "\n" + theirs + "\n")},
		{Code: 0},
		{Code: 0, Stdout: []byte(theirs + "\n" + moved + "\n")},
		{Code: 0},
	}}
	b := cliBackend(runner, &countingPrompter{password: "p"})

	require.NoError(t, b.Delete(t.Context(), service))

	require.Len(t, runner.Calls, 4, "one entry moved, so one entry is removed again")
	last := runner.Calls[len(runner.Calls)-1]
	assert.Contains(t, last.Args, moved, "the copy the bin kept is the one to remove")
	assert.NotContains(t, last.Args, theirs,
		"an entry of the same name that this deletion did not move is somebody else's")
}

// TestKeePassXCCLIDeleteStopsWhereThereIsNoRecycleBin covers the database whose
// owner turned the bin off, or which never had one: `rm` deletes outright,
// nothing appears anywhere, and there is nothing more to do. A second removal
// there would be aimed at nothing.
func TestKeePassXCCLIDeleteStopsWhereThereIsNoRecycleBin(t *testing.T) {
	const service = wallet.DefaultServicePrefix + "-id_ed25519"
	runner := &runtest.Recorder{Results: []run.Result{
		{Code: 0, Stdout: []byte("/SSHakku/" + service + "\n")},
		{Code: 0},
		{Code: 1}, // search finds nothing, which is how it reports a miss.
	}}
	b := cliBackend(runner, &countingPrompter{password: "p"})

	require.NoError(t, b.Delete(t.Context(), service))

	assert.Len(t, runner.Calls, 3, "with nothing left to find there is nothing left to remove")
}

// TestKeePassXCCLIDeleteReportsEveryWayTheRemovalCanGoWrong walks the failures
// that can arrive after the entry has been found. They differ in where they
// happen — the removal, the search that follows the move, the removal of what
// the bin kept — and in all of them the passphrase is still in the database, so
// none of them may come back as a deletion that worked.
func TestKeePassXCCLIDeleteReportsEveryWayTheRemovalCanGoWrong(t *testing.T) {
	const service = wallet.DefaultServicePrefix + "-id_ed25519"
	const moved = "/Papierkorb/" + service
	found := run.Result{Code: 0, Stdout: []byte("/SSHakku/" + service + "\n")}
	refusal := run.Result{Code: 1, Stderr: []byte("Failed to open the terminal for the password\n")}
	gone := run.Result{Code: 0, Stdout: []byte(moved + "\n")}

	for _, tc := range []struct {
		name    string
		results []run.Result
		errs    []error
		want    error
		because string
	}{
		{
			name:    "the removal will not run",
			results: []run.Result{found},
			errs:    []error{nil, errKeepassxcCliVanished},
			want:    errKeepassxcCliVanished,
			because: "a keepassxc-cli that could not be started has removed nothing",
		},
		{
			name:    "the removal is refused the password",
			results: []run.Result{found, refusal},
			want:    ErrPasswordNotAccepted,
			because: "a database that would not take the password still holds the entry",
		},
		{
			name:    "the search for what the bin kept will not run",
			results: []run.Result{found, {Code: 0}},
			errs:    []error{nil, nil, errKeepassxcCliVanished},
			want:    errKeepassxcCliVanished,
			because: "where the entry went is unknown, so the copy the bin kept cannot be said to be gone",
		},
		{
			name:    "the removal of the copy the bin kept will not run",
			results: []run.Result{found, {Code: 0}, gone},
			errs:    []error{nil, nil, nil, errKeepassxcCliVanished},
			want:    errKeepassxcCliVanished,
			because: "the entry was moved and not deleted, which is the case this second removal exists for",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := &runtest.Recorder{Results: tc.results, Errs: tc.errs}
			b := cliBackend(runner, &countingPrompter{password: "p"})

			assert.ErrorIs(t, b.Delete(t.Context(), service), tc.want, tc.because)
		})
	}
}

// TestKeePassXCCLIDeleteOfAnEntryRemovedMeanwhileIsSuccess is the entry that
// was there when the search ran and gone by the time the removal reached it —
// another session, or the user in the KeePassXC window. What was asked for has
// happened, and there is then nothing the bin can be holding.
func TestKeePassXCCLIDeleteOfAnEntryRemovedMeanwhileIsSuccess(t *testing.T) {
	const service = wallet.DefaultServicePrefix + "-id_ed25519"
	runner := &runtest.Recorder{Results: []run.Result{
		{Code: 0, Stdout: []byte("/SSHakku/" + service + "\n")},
		{Code: 1, Stderr: []byte("Entry /SSHakku/" + service + " not found.\n")},
	}}
	b := cliBackend(runner, &countingPrompter{password: "p"})

	require.NoError(t, b.Delete(t.Context(), service),
		"an entry that is already gone is the outcome forget was asked for")
	assert.Len(t, runner.Calls, 2, "nothing was moved, so there is nowhere to look and nothing to remove again")
}

func TestKeePassXCCLIDeleteOfAnAbsentEntryIsSuccess(t *testing.T) {
	runner := &runtest.Recorder{Results: []run.Result{{Code: 1, Stderr: []byte("Entry not found\n")}}}
	b := cliBackend(runner, &countingPrompter{password: "p"})

	require.NoError(t, b.Delete(t.Context(), wallet.DefaultServicePrefix+"-gone"),
		"a passphrase that is already not there is the outcome that was asked for")
}

func TestKeePassXCCLIListsWhatItStored(t *testing.T) {
	runner := &runtest.Recorder{Results: []run.Result{{
		Code:   0,
		Stdout: []byte(wallet.DefaultServicePrefix + "-id_ed25519\n" + wallet.DefaultServicePrefix + "-id_rsa\nnested/\n\n"),
	}}}
	b := cliBackend(runner, &countingPrompter{password: "p"})

	got, err := b.List(t.Context())
	require.NoError(t, err, "listing what the database holds must succeed")
	assert.Equal(t, []string{wallet.DefaultServicePrefix + "-id_ed25519", wallet.DefaultServicePrefix + "-id_rsa"}, got,
		"every entry must be named, and a trailing slash marks a group rather than an entry")
}

func TestKeePassXCCLIWithNoDatabaseSaysSo(t *testing.T) {
	b := &CLI{Runner: &runtest.Recorder{}, Prompter: &countingPrompter{password: "p"}}
	_, _, err := b.Lookup(t.Context(), wallet.DefaultServicePrefix+"-id_ed25519")
	assert.ErrorIs(t, err, ErrNoDatabase,
		"a route with no database configured must say which piece is missing, not fail as an empty wallet")
}

func TestKeePassXCCLIPassesTheKeyFileWhenConfigured(t *testing.T) {
	runner := &runtest.Recorder{Results: []run.Result{{Code: 0, Stdout: []byte("x\n")}}}
	b := cliBackend(runner, &countingPrompter{password: "p"})
	b.KeyFile = "/db.key"

	_, _, err := b.Lookup(t.Context(), wallet.DefaultServicePrefix+"-id_ed25519")
	require.NoError(t, err, "reading an entry out of a key-file-protected database must succeed")
	require.NotEmpty(t, runner.Calls, "the database must actually be asked")
	assert.Contains(t, strings.Join(runner.Calls[0].Args, " "), "--key-file /db.key",
		"a database that also needs a key file cannot be opened without one")
}

// TestKeePassXCCLIKeyFileOnlyDatabaseAsksNothing covers a database whose key is
// a key file and nothing else. There is no password on it, so there is nothing
// to ask anybody — at this login or at any later one — and the route becomes as
// silent as one talking to a KeePassXC that is already open.
//
// Which way the database is locked is the configuration's to state and never
// SSHakku's to work out: a database can carry both a key file and a password,
// so a build that read keepassxc_key_file as meaning "no password" would shut
// those users out of their own wallet with a refusal they never asked for.
func TestKeePassXCCLIKeyFileOnlyDatabaseAsksNothing(t *testing.T) {
	runner := &runtest.Recorder{Results: []run.Result{{Code: 0, Stdout: []byte("passphrase\n")}}}
	prompter := &countingPrompter{password: "never-needed"}
	b := cliBackend(runner, prompter)
	b.KeyFile = "/db.keyx"
	b.NoPassword = true

	got, found, err := b.Lookup(t.Context(), wallet.DefaultServicePrefix+"-id_ed25519")

	require.NoError(t, err, "a database that opens on its key file must open")
	assert.True(t, found, "the entry is there and must be reported as found")
	assert.Equal(t, "passphrase", got, "and the passphrase must come back whole")
	assert.Zero(t, prompter.asked,
		"there is no password on this database, so nobody may be asked for one")
	require.Len(t, runner.Calls, 1, "one lookup is one command")
	assert.Contains(t, runner.Calls[0].Args, "--no-password",
		"keepassxc-cli waits for a password unless it is told the database has none")
	assert.Empty(t, runner.Calls[0].Stdin,
		"a password line where there is no password is read as an answer to the next question")
}

// TestKeePassXCCLIKeyFileOnlyStoreSendsOnlyTheEntryPassword is the same rule
// where there is a second secret in the exchange. The entry's own password is
// still fed on standard input; what goes away is the database password line in
// front of it, which keepassxc-cli would otherwise read as the entry's.
func TestKeePassXCCLIKeyFileOnlyStoreSendsOnlyTheEntryPassword(t *testing.T) {
	const passphrase = "the-key-passphrase"
	runner := &runtest.Recorder{Results: []run.Result{
		{Code: 1}, // the lookup that decides between add and edit: not there yet.
		{Code: 0}, // mkdir.
		{Code: 0}, // add.
	}}
	prompter := &countingPrompter{password: "never-needed"}
	b := cliBackend(runner, prompter)
	b.KeyFile = "/db.keyx"
	b.NoPassword = true

	require.NoError(t, b.Store(t.Context(), wallet.DefaultServicePrefix+"-id_ed25519", "id_ed25519", passphrase))

	assert.Zero(t, prompter.asked, "storing must not ask for a password the database has not got")
	last := runner.Calls[len(runner.Calls)-1]
	assert.Equal(t, passphrase+"\n", last.Stdin,
		"the entry's password is the whole of what this command has to read")
}

// TestKeePassXCCLIAPasswordProtectedDatabaseIsStillAskedAbout guards the
// default from the case above: --no-password on a database that has one turns
// every operation into a refusal, so it must appear nowhere unless it was asked
// for by name.
func TestKeePassXCCLIAPasswordProtectedDatabaseIsStillAskedAbout(t *testing.T) {
	runner := &runtest.Recorder{Results: []run.Result{{Code: 0, Stdout: []byte("x\n")}}}
	prompter := &countingPrompter{password: "the-database-password"}
	b := cliBackend(runner, prompter)
	b.KeyFile = "/db.keyx"

	_, _, err := b.Lookup(t.Context(), wallet.DefaultServicePrefix+"-id_ed25519")

	require.NoError(t, err, "reading an entry out of a password-protected database must succeed")
	assert.Equal(t, 1, prompter.asked, "a database with a password on it has to be asked about")
	require.Len(t, runner.Calls, 1, "one lookup is one command")
	assert.NotContains(t, runner.Calls[0].Args, "--no-password",
		"a key file beside a password does not mean the password is gone")
	assert.Contains(t, runner.Calls[0].Stdin, "the-database-password",
		"and the password still has to reach the command that needs it")
}

func TestKeePassXCCLIReportsARefusedPrompt(t *testing.T) {
	refused := errTheUserDismissedTheDialog
	b := cliBackend(&runtest.Recorder{}, &countingPrompter{err: refused})

	_, _, err := b.Lookup(t.Context(), wallet.DefaultServicePrefix+"-id_ed25519")
	assert.ErrorIs(t, err, refused, "a user who dismissed the prompt has answered, and must be obeyed")
}

// runnerErr is a runner that cannot start the command at all — keepassxc-cli
// not installed, for instance, which is a different thing from it failing.
func runnerErr(n int) *runtest.Recorder {
	r := &runtest.Recorder{}
	for range n {
		r.Errs = append(r.Errs, errKeepassxcCliNoSuchFile)
	}
	return r
}

func TestKeePassXCCLIReportsACommandThatCannotRun(t *testing.T) {
	for _, tc := range []struct {
		name string
		call func(*CLI) error
	}{
		{"lookup", func(b *CLI) error {
			_, _, err := b.Lookup(t.Context(), wallet.DefaultServicePrefix+"-k")
			return err
		}},
		{"store", func(b *CLI) error { return b.Store(t.Context(), wallet.DefaultServicePrefix+"-k", "", "p") }},
		{"delete", func(b *CLI) error { return b.Delete(t.Context(), wallet.DefaultServicePrefix+"-k") }},
		{"list", func(b *CLI) error { _, err := b.List(t.Context()); return err }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := cliBackend(runnerErr(3), &countingPrompter{password: "p"})
			assert.Error(t, tc.call(b), "a command that could not run must be reported, not read as an empty wallet")
		})
	}
}

func TestKeePassXCCLIStoreReportsAFailureItCannotName(t *testing.T) {
	runner := &runtest.Recorder{Results: []run.Result{
		{Code: 1},
		{Code: 0},
		{Code: 1, Stderr: []byte("Group SSHakku not found\nsecond line\n")},
	}}
	b := cliBackend(runner, &countingPrompter{password: "p"})

	err := b.Store(t.Context(), wallet.DefaultServicePrefix+"-k", "", "x")
	require.Error(t, err, "a write that failed must be reported, not passed over")
	assert.Contains(t, err.Error(), "Group SSHakku not found",
		"and it must carry KeePassXC's own words, which are the only account of what went wrong")
	assert.NotContains(t, err.Error(), "second line",
		"one line of it, so the reason fits where a user reads it")
}

func TestKeePassXCCLIReportsARefusedPasswordOnEveryOperation(t *testing.T) {
	refusal := run.Result{Code: 1, Stderr: []byte("Failed to open the terminal for the password\n")}

	t.Run("store", func(t *testing.T) {
		// The existence check misses cleanly and the group is made; the write
		// is what is refused.
		runner := &runtest.Recorder{Results: []run.Result{{Code: 1}, {Code: 0}, refusal}}
		b := cliBackend(runner, &countingPrompter{password: "p"})
		assert.ErrorIs(t, b.Store(t.Context(), wallet.DefaultServicePrefix+"-k", "", "x"), ErrPasswordNotAccepted,
			"a database that would not take the password must be said so")
	})

	t.Run("delete", func(t *testing.T) {
		b := cliBackend(&runtest.Recorder{Results: []run.Result{refusal}}, &countingPrompter{password: "p"})
		assert.ErrorIs(t, b.Delete(t.Context(), wallet.DefaultServicePrefix+"-k"), ErrPasswordNotAccepted,
			"a database that would not take the password must be said so, or forget claims to have removed nothing")
	})

	t.Run("list", func(t *testing.T) {
		b := cliBackend(&runtest.Recorder{Results: []run.Result{refusal}}, &countingPrompter{password: "p"})
		_, err := b.List(t.Context())
		assert.ErrorIs(t, err, ErrPasswordNotAccepted,
			"a database that would not take the password must be said so, not listed as empty")
	})
}

func TestKeePassXCCLIListOfAnAbsentGroupIsEmpty(t *testing.T) {
	runner := &runtest.Recorder{Results: []run.Result{{Code: 1, Stderr: []byte("Cannot find group SSHakku\n")}}}
	b := cliBackend(runner, &countingPrompter{password: "p"})

	got, err := b.List(t.Context())
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
	runner := &runtest.Recorder{
		Results: []run.Result{{Code: 1}, {Code: 0}},
		Errs:    []error{nil, nil, errKeepassxcCliVanished},
	}
	b := cliBackend(runner, &countingPrompter{password: "p"})

	assert.Error(t, b.Store(t.Context(), wallet.DefaultServicePrefix+"-k", "", "x"),
		"a write that could not start must be reported, not read as saved")
}

// TestKeePassXCCLIStoreReportsAGroupThatCannotBeMade covers the step a real
// keepassxc-cli made necessary: without the group, the add cannot land.
func TestKeePassXCCLIStoreReportsAGroupThatCannotBeMade(t *testing.T) {
	runner := &runtest.Recorder{
		Results: []run.Result{{Code: 1}},
		Errs:    []error{nil, errKeepassxcCliVanished},
	}
	b := cliBackend(runner, &countingPrompter{password: "p"})

	assert.Error(t, b.Store(t.Context(), wallet.DefaultServicePrefix+"-k", "", "x"),
		"a group that could not be made must be reported: nothing can be written into it")
}
