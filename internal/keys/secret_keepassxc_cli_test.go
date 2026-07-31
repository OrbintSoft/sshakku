package keys

import (
	"errors"
	"strings"
	"testing"
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

	if _, _, err := b.Lookup("SSH-Key-id_ed25519"); err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("ran %d commands, want 1", len(runner.calls))
	}
	call := runner.calls[0]
	for _, arg := range call.Args {
		if strings.Contains(arg, password) {
			t.Fatalf("the database password appeared in argv: %v", call.Args)
		}
	}
	if !strings.Contains(call.Stdin, password) {
		t.Error("the database password must be fed on standard input, or the command cannot open the database")
	}
}

// TestKeePassXCCLIStoreKeepsThePassphraseOffArgvToo covers the second secret
// the CLI needs: the key's own passphrase, which follows the database password
// on standard input.
func TestKeePassXCCLIStoreKeepsThePassphraseOffArgvToo(t *testing.T) {
	const dbPassword = "db-password"
	const passphrase = "the-key-passphrase"
	runner := &recordingRunner{results: []Result{
		{Code: 1}, // the existence check: no such entry yet
		{Code: 0}, // the add
	}}
	b := cliBackend(runner, &countingPrompter{password: dbPassword})

	if err := b.Store("SSH-Key-id_ed25519", "", passphrase); err != nil {
		t.Fatalf("Store: %v", err)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("ran %d commands, want a lookup then a write", len(runner.calls))
	}
	write := runner.calls[1]
	for _, arg := range write.Args {
		if strings.Contains(arg, passphrase) || strings.Contains(arg, dbPassword) {
			t.Fatalf("a secret appeared in argv: %v", write.Args)
		}
	}
	if write.Stdin != dbPassword+"\n"+passphrase+"\n" {
		t.Errorf("stdin = %q, want the database password then the passphrase, in the order asked for", write.Stdin)
	}
}

func TestKeePassXCCLIStoreAddsThenEdits(t *testing.T) {
	t.Run("a key with no entry yet is added", func(t *testing.T) {
		runner := &recordingRunner{results: []Result{{Code: 1}, {Code: 0}}}
		b := cliBackend(runner, &countingPrompter{password: "p"})
		if err := b.Store("SSH-Key-new", "", "x"); err != nil {
			t.Fatalf("Store: %v", err)
		}
		if runner.calls[1].Args[0] != "add" {
			t.Errorf("verb = %q, want add", runner.calls[1].Args[0])
		}
	})

	t.Run("a key already stored is edited in place", func(t *testing.T) {
		runner := &recordingRunner{results: []Result{
			{Code: 0, Stdout: []byte("old\n")},
			{Code: 0},
		}}
		b := cliBackend(runner, &countingPrompter{password: "p"})
		if err := b.Store("SSH-Key-existing", "", "x"); err != nil {
			t.Fatalf("Store: %v", err)
		}
		if runner.calls[1].Args[0] != "edit" {
			t.Errorf("verb = %q, want edit — adding again would leave a second copy of the secret", runner.calls[1].Args[0])
		}
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

	if _, _, err := b.Lookup("SSH-Key-one"); err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if _, _, err := b.Lookup("SSH-Key-two"); err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if prompter.asked != 1 {
		t.Errorf("asked %d times, want once for the whole process", prompter.asked)
	}
}

func TestKeePassXCCLILookupOfAnAbsentEntryIsAMiss(t *testing.T) {
	runner := &recordingRunner{results: []Result{{Code: 1, Stderr: []byte("Could not find entry\n")}}}
	b := cliBackend(runner, &countingPrompter{password: "p"})

	_, found, err := b.Lookup("SSH-Key-absent")
	if err != nil {
		t.Fatalf("a miss must not be an error: %v", err)
	}
	if found {
		t.Error("nothing stored means nothing found")
	}
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

	if _, _, err := b.Lookup("SSH-Key-id_ed25519"); !errors.Is(err, ErrPasswordNotAccepted) {
		t.Fatalf("err = %v, want ErrPasswordNotAccepted — a refused interface is not an empty wallet", err)
	}
}

// TestKeePassXCCLICanDelete is the difference from the local-protocol route,
// which has no verb for it: here `sshakku forget` really removes the entry.
func TestKeePassXCCLICanDelete(t *testing.T) {
	runner := &recordingRunner{results: []Result{{Code: 0}}}
	b := cliBackend(runner, &countingPrompter{password: "p"})

	if err := b.Delete("SSH-Key-id_ed25519"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if runner.calls[0].Args[0] != "rm" {
		t.Errorf("verb = %q, want rm", runner.calls[0].Args[0])
	}
	if !strings.Contains(strings.Join(runner.calls[0].Args, " "), "SSHakku/SSH-Key-id_ed25519") {
		t.Errorf("args = %v, want the entry path inside the SSHakku group", runner.calls[0].Args)
	}
}

func TestKeePassXCCLIDeleteOfAnAbsentEntryIsSuccess(t *testing.T) {
	runner := &recordingRunner{results: []Result{{Code: 1, Stderr: []byte("Entry not found\n")}}}
	b := cliBackend(runner, &countingPrompter{password: "p"})

	if err := b.Delete("SSH-Key-gone"); err != nil {
		t.Fatalf("forgetting an already-forgotten key must succeed: %v", err)
	}
}

func TestKeePassXCCLIListsWhatItStored(t *testing.T) {
	runner := &recordingRunner{results: []Result{{
		Code:   0,
		Stdout: []byte("SSH-Key-id_ed25519\nSSH-Key-id_rsa\nnested/\n\n"),
	}}}
	b := cliBackend(runner, &countingPrompter{password: "p"})

	got, err := b.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"SSH-Key-id_ed25519", "SSH-Key-id_rsa"}
	if len(got) != len(want) {
		t.Fatalf("List = %v, want %v — a trailing slash marks a group, not an entry", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("entry %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestKeePassXCCLIWithNoDatabaseSaysSo(t *testing.T) {
	b := &KeePassXCCLIBackend{Runner: &recordingRunner{}, Prompter: &countingPrompter{password: "p"}}
	if _, _, err := b.Lookup("SSH-Key-id_ed25519"); !errors.Is(err, ErrNoDatabase) {
		t.Fatalf("err = %v, want ErrNoDatabase", err)
	}
}

func TestKeePassXCCLIPassesTheKeyFileWhenConfigured(t *testing.T) {
	runner := &recordingRunner{results: []Result{{Code: 0, Stdout: []byte("x\n")}}}
	b := cliBackend(runner, &countingPrompter{password: "p"})
	b.KeyFile = "/db.key"

	if _, _, err := b.Lookup("SSH-Key-id_ed25519"); err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	joined := strings.Join(runner.calls[0].Args, " ")
	if !strings.Contains(joined, "--key-file /db.key") {
		t.Errorf("args = %v, want the key file passed through", runner.calls[0].Args)
	}
}

func TestKeePassXCCLIReportsARefusedPrompt(t *testing.T) {
	refused := errors.New("the user dismissed the dialog")
	b := cliBackend(&recordingRunner{}, &countingPrompter{err: refused})

	if _, _, err := b.Lookup("SSH-Key-id_ed25519"); !errors.Is(err, refused) {
		t.Fatalf("err = %v, want the refusal", err)
	}
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
		{"lookup", func(b *KeePassXCCLIBackend) error { _, _, err := b.Lookup("SSH-Key-k"); return err }},
		{"store", func(b *KeePassXCCLIBackend) error { return b.Store("SSH-Key-k", "", "p") }},
		{"delete", func(b *KeePassXCCLIBackend) error { return b.Delete("SSH-Key-k") }},
		{"list", func(b *KeePassXCCLIBackend) error { _, err := b.List(); return err }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := cliBackend(runnerErr(2), &countingPrompter{password: "p"})
			if err := tc.call(b); err == nil {
				t.Fatal("a command that could not run must be reported, not read as an empty wallet")
			}
		})
	}
}

func TestKeePassXCCLIStoreReportsAFailureItCannotName(t *testing.T) {
	runner := &recordingRunner{results: []Result{
		{Code: 1},
		{Code: 1, Stderr: []byte("Group SSHakku not found\nsecond line\n")},
	}}
	b := cliBackend(runner, &countingPrompter{password: "p"})

	err := b.Store("SSH-Key-k", "", "x")
	if err == nil {
		t.Fatal("a write that failed must be reported")
	}
	if !strings.Contains(err.Error(), "Group SSHakku not found") {
		t.Errorf("err = %v, want KeePassXC's own words", err)
	}
	if strings.Contains(err.Error(), "second line") {
		t.Errorf("err = %v, want a single line", err)
	}
}

func TestKeePassXCCLIReportsARefusedPasswordOnEveryOperation(t *testing.T) {
	refusal := Result{Code: 1, Stderr: []byte("Failed to open the terminal for the password\n")}

	t.Run("store", func(t *testing.T) {
		// The existence check misses cleanly; the write is what is refused.
		runner := &recordingRunner{results: []Result{{Code: 1}, refusal}}
		b := cliBackend(runner, &countingPrompter{password: "p"})
		if err := b.Store("SSH-Key-k", "", "x"); !errors.Is(err, ErrPasswordNotAccepted) {
			t.Fatalf("err = %v, want ErrPasswordNotAccepted", err)
		}
	})

	t.Run("delete", func(t *testing.T) {
		b := cliBackend(&recordingRunner{results: []Result{refusal}}, &countingPrompter{password: "p"})
		if err := b.Delete("SSH-Key-k"); !errors.Is(err, ErrPasswordNotAccepted) {
			t.Fatalf("err = %v, want ErrPasswordNotAccepted — otherwise forget would claim success", err)
		}
	})

	t.Run("list", func(t *testing.T) {
		b := cliBackend(&recordingRunner{results: []Result{refusal}}, &countingPrompter{password: "p"})
		if _, err := b.List(); !errors.Is(err, ErrPasswordNotAccepted) {
			t.Fatalf("err = %v, want ErrPasswordNotAccepted", err)
		}
	})
}

func TestKeePassXCCLIListOfAnAbsentGroupIsEmpty(t *testing.T) {
	runner := &recordingRunner{results: []Result{{Code: 1, Stderr: []byte("Cannot find group SSHakku\n")}}}
	b := cliBackend(runner, &countingPrompter{password: "p"})

	got, err := b.List()
	if err != nil {
		t.Fatalf("nothing stored yet must not be an error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("List = %v, want nothing", got)
	}
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
			if got := firstLine([]byte(tc.in)); got != tc.want {
				t.Errorf("firstLine(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestKeePassXCCLIStoreReportsAWriteThatCannotRun covers the case where the
// existence check works and the write itself cannot start.
func TestKeePassXCCLIStoreReportsAWriteThatCannotRun(t *testing.T) {
	runner := &recordingRunner{
		results: []Result{{Code: 1}},
		errs:    []error{nil, errors.New("keepassxc-cli vanished")},
	}
	b := cliBackend(runner, &countingPrompter{password: "p"})

	if err := b.Store("SSH-Key-k", "", "x"); err == nil {
		t.Fatal("a write that could not start must be reported")
	}
}
