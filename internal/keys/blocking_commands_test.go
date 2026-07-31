package keys

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// blockingTools puts a never-answering stand-in ahead of every external program
// this package runs, for the calling test only. It stands in for a tool that
// cannot answer and cannot fail either: a wallet locked behind an unlock prompt
// nobody is there to answer, an X server that accepted the connection and went
// quiet, a CLI waiting on a network that has gone away.
//
// PATH is the whole seam — every one of these is resolved by name — so nothing
// about the components under test is replaced.
func blockingTools(t *testing.T) {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("..", "..", "test", "bats", "fixtures", "blocking-secret-tool"))
	if err != nil {
		t.Fatalf("read the blocking tool fixture: %v", err)
	}
	dir := t.TempDir()
	for _, bin := range []string{secretToolBin, bitwardenBin, onePasswordBin, kdialogBin, "xset"} {
		if err := os.WriteFile(filepath.Join(dir, bin), src, 0o755); err != nil {
			t.Fatalf("install the blocking %s: %v", bin, err)
		}
	}
	// Prepended, not substituted: the fixture is a shell script and still needs
	// the ordinary PATH to find the tools it runs.
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// fixedPrompter answers instantly, so a Bitwarden call reaches the bw CLI —
// which is what this test is about — instead of stopping at the master-password
// prompt, which is a different question.
type fixedPrompter struct{}

func (fixedPrompter) Prompt(string) (string, error) { return "master-password", nil }
func (fixedPrompter) Available() bool               { return true }

// TestNoCommandBlocksIndefinitely verifies that nothing SSHakku runs can hold a
// shell up forever. A login shell and an `ssh` awaiting a passphrase both sit
// behind these calls, so an external program that never answers must become an
// error the caller can act on — falling back to the terminal, or reporting the
// backend unavailable — rather than silence with no end.
//
// The bound asserted here is deliberately loose. It is not a performance
// budget: any finite answer proves a deadline exists, and no answer at all is
// the defect.
func TestNoCommandBlocksIndefinitely(t *testing.T) {
	const patience = 30 * time.Second

	// The components that wait on a person default to a budget measured in
	// minutes, which is right for them and useless here: what this test asks is
	// whether every call site honours a budget at all, so they are built with a
	// short one. That the defaults themselves are finite is TestTimeoutDefaults.
	const brief = 300 * time.Millisecond

	blockingTools(t)

	for _, tc := range []struct {
		name string
		call func()
	}{
		// Left on the bare default on purpose: this one witnesses the structural
		// net, that a call site choosing no budget still gets a finite one.
		{"GUI detection (xset)", func() {
			GUIAvailable(GUIEnv{Display: ":0"}, ExecRunner{}, KDialogPrompter{})
		}},
		{"graphical passphrase prompt (kdialog)", func() {
			_, _ = KDialogPrompter{Runner: ExecRunner{}, Timeout: brief}.Prompt("id_test")
		}},
		{"secret-tool Lookup", func() {
			_, _, _ = SecretToolBackend{Runner: ExecRunner{}, User: "u", Timeout: brief}.Lookup("SSH-Key-id_test")
		}},
		{"secret-tool Store", func() {
			_ = SecretToolBackend{Runner: ExecRunner{}, User: "u", Timeout: brief}.Store("SSH-Key-id_test", "label", "s3cret")
		}},
		{"secret-tool Delete", func() {
			_ = SecretToolBackend{Runner: ExecRunner{}, User: "u", Timeout: brief}.Delete("SSH-Key-id_test")
		}},
		{"1Password Lookup", func() {
			_, _, _ = (&OnePasswordBackend{Runner: ExecRunner{}, Vault: "sshakku", Timeout: brief}).Lookup("SSH-Key-id_test")
		}},
		{"Bitwarden Lookup", func() {
			_, _, _ = (&BitwardenBackend{
				Runner: ExecRunner{}, Prompter: fixedPrompter{}, Email: "u@example.invalid", Timeout: brief,
			}).Lookup("SSH-Key-id_test")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			done := make(chan struct{})
			go func() {
				defer close(done)
				tc.call()
			}()

			select {
			case <-done:
			case <-time.After(patience):
				t.Fatalf("%s never returned against a program that does not answer", tc.name)
			}
		})
	}
}

// TestTimeoutDefaults guards the other half of the guarantee: a call site that
// chooses no budget must still get a finite one. A zero default would restore
// the unbounded wait everywhere at once, and every other test here would still
// pass, since they all pass a budget of their own.
func TestTimeoutDefaults(t *testing.T) {
	if DefaultCommandTimeout <= 0 {
		t.Errorf("DefaultCommandTimeout = %v, want a finite bound", DefaultCommandTimeout)
	}
	if DefaultInteractiveTimeout <= 0 {
		t.Errorf("DefaultInteractiveTimeout = %v, want a finite bound", DefaultInteractiveTimeout)
	}
}
