package keys

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecRunnerRun(t *testing.T) {
	t.Run("captures stdout, stderr, and exit code", func(t *testing.T) {
		res, err := ExecRunner{}.Run(t.Context(), Cmd{Name: "sh", Args: []string{"-c", "echo out; echo err >&2; exit 3"}})
		require.NoError(t, err, "a command that ran and exited non-zero is not a failure to run it")
		assert.Equal(t, "out", strings.TrimSpace(string(res.Stdout)), "what it printed is what a wallet answered")
		assert.Equal(t, "err", strings.TrimSpace(string(res.Stderr)), "and what it complained is the reason a user reads")
		assert.Equal(t, 3, res.Code, "the exit code decides whether that was a miss or a refusal")
	})

	t.Run("zero Timeout does not bound the command", func(t *testing.T) {
		res, err := ExecRunner{}.Run(t.Context(), Cmd{Name: "sh", Args: []string{"-c", "sleep 0.2; echo done"}})
		require.NoError(t, err, "running a command with no deadline must succeed")
		assert.Equal(t, "done", strings.TrimSpace(string(res.Stdout)),
			"a caller that named no budget gets none imposed here; the budgets belong to the call sites")
	})

	t.Run("a positive Timeout kills a command that outlives it", func(t *testing.T) {
		// Runs sleep directly, not via a shell: a shell wrapping the last
		// command may or may not exec-replace itself depending on the shell
		// and environment, which would leave the kill racing an unrelated
		// process tree instead of the one under test.
		start := time.Now()
		res, err := ExecRunner{}.Run(t.Context(), Cmd{Name: "sleep", Args: []string{"5"}, Timeout: 100 * time.Millisecond})
		require.NoError(t, err, "a command that outlived its budget is still a command that ran")
		assert.Less(t, time.Since(start), 2*time.Second,
			"and it must be cut short well before its own five seconds, or nothing is waiting on the budget")
		assert.NotZero(t, res.Code, "a process that was killed did not succeed, and must not be read as having done so")
	})

	t.Run("a command that finishes within its Timeout completes normally", func(t *testing.T) {
		res, err := ExecRunner{}.Run(t.Context(), Cmd{Name: "sh", Args: []string{"-c", "echo fast"}, Timeout: 5 * time.Second})
		require.NoError(t, err, "a command that answered in time must succeed")
		assert.Equal(t, "fast", strings.TrimSpace(string(res.Stdout)),
			"and a budget nobody reached must not change what it said")
	})
}
