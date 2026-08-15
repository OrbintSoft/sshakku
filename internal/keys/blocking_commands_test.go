package keys

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/keys/wallet"
	"github.com/OrbintSoft/sshakku/internal/run"
)

// blockingTools puts a never-answering stand-in ahead of every external program
// this package runs, for the calling test only. It stands in for a tool that
// cannot answer and cannot fail either: a wallet locked behind an unlock prompt
// nobody is there to answer, an X server that accepted the connection and went
// quiet, a CLI waiting on a network that has gone away.
//
// PATH is the whole seam — every one of these is resolved by name — so nothing
// about the components under test is replaced. Which programs those are differs
// per platform, so each names its own (platformBlockingTools).
func blockingTools(t *testing.T) {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("..", "..", "test", "bats", "fixtures", "blocking-secret-tool"))
	require.NoError(t, err, "a stand-in that neither answers nor fails")
	dir := t.TempDir()
	// Named here rather than taken from the constants the wallets resolve them
	// by: this is the list of programs SSHakku is claimed to run, and one derived
	// from the implementation would agree with it whatever it became.
	tools := append([]string{"bw", "op"}, platformBlockingTools()...)
	for _, bin := range tools {
		require.NoErrorf(t, os.WriteFile(filepath.Join(dir, bin), src, 0o755), "install the blocking %s", bin)
	}
	// Prepended, not substituted: the fixture is a shell script and still needs
	// the ordinary PATH to find the tools it runs.
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// fixedPrompter answers instantly, so a Bitwarden call reaches the bw CLI —
// which is what this test is about — instead of stopping at the master-password
// prompt, which is a different question.
type fixedPrompter struct{}

func (fixedPrompter) Prompt(context.Context, string) (string, error) { return "master-password", nil }
func (fixedPrompter) Available(context.Context) bool                 { return true }

// TestNoCommandBlocksIndefinitely verifies that nothing SSHakku runs can hold a
// shell up forever. A login shell and an `ssh` awaiting a passphrase both sit
// behind these calls, so an external program that never answers must become an
// error the caller can act on — falling back to the terminal, or reporting the
// backend unavailable — rather than silence with no end.
//
// The bound asserted here is deliberately loose. It is not a performance
// budget: any finite answer proves a deadline exists, and no answer at all is
// the defect.
// blockingCase is one call that must come back, and the name it is reported
// under. Platform-specific ones are contributed by platformBlockingCases.
type blockingCase struct {
	name string
	call func()
}

func TestNoCommandBlocksIndefinitely(t *testing.T) {
	const patience = 30 * time.Second

	// The components that wait on a person default to a budget measured in
	// minutes, which is right for them and useless here: what this test asks is
	// whether every call site honours a budget at all, so they are built with a
	// short one. That the defaults themselves are finite is TestTimeoutDefaults.
	const brief = 300 * time.Millisecond

	blockingTools(t)

	cases := []blockingCase{
		{"1Password Lookup", func() {
			_, _, _ = (&wallet.OnePassword{Runner: run.ExecRunner{}, Vault: "sshakku", Timeout: brief}).Lookup(t.Context(), wallet.DefaultServicePrefix+"-id_test")
		}},
		{"Bitwarden Lookup", func() {
			_, _, _ = (&wallet.Bitwarden{
				Runner: run.ExecRunner{}, Prompter: fixedPrompter{}, Email: "u@example.invalid", Timeout: brief,
			}).Lookup(t.Context(), wallet.DefaultServicePrefix+"-id_test")
		}},
	}
	// The wallets each platform reaches by running a program differ, and so does
	// which of them exist at all; each platform names its own.
	cases = append(cases, platformBlockingCases(t.Context(), brief)...)

	for _, tc := range cases {
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
				assert.Failf(t, "the call never came back",
					"%s waited on a program that does not answer, and a login shell or an ssh at a passphrase "+
						"prompt is sitting behind it", tc.name)
			}
		})
	}
}
