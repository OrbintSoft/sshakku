package run

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/testproc"
)

// The program these cases run is this test binary, re-entered — see
// internal/testproc. A runner that spawns processes cannot be shown to work
// against a stand-in for spawning, but it does not need any particular program
// to spawn, and borrowing the machine's sh or sleep would make these assertions
// depend on which tools the system happens to carry.
func TestExecRunnerRun(t *testing.T) {
	t.Run("captures stdout, stderr, and exit code", func(t *testing.T) {
		name, args := testproc.Command(t, testproc.Emit, "out", "err", "3")
		res, err := ExecRunner{}.Run(t.Context(), Cmd{Name: name, Args: args})
		require.NoError(t, err, "a command that ran and exited non-zero is not a failure to run it")
		assert.Equal(t, "out", string(res.Stdout), "what it printed is what a wallet answered")
		assert.Equal(t, "err", string(res.Stderr), "and what it complained is the reason a user reads")
		assert.Equal(t, 3, res.Code, "the exit code decides whether that was a miss or a refusal")
	})

	t.Run("zero Timeout does not bound the command", func(t *testing.T) {
		name, args := testproc.Command(t, testproc.Sleep, "200ms")
		start := time.Now()
		res, err := ExecRunner{}.Run(t.Context(), Cmd{Name: name, Args: args})
		require.NoError(t, err, "running a command with no deadline must succeed")
		assert.Zero(t, res.Code, "and it must be allowed to finish, not cut short")
		assert.GreaterOrEqual(t, time.Since(start), 200*time.Millisecond,
			"a caller that named no budget gets none imposed here; the budgets belong to the call sites")
	})

	t.Run("a positive Timeout kills a command that outlives it", func(t *testing.T) {
		name, args := testproc.Command(t, testproc.Sleep, "5s")
		start := time.Now()
		res, err := ExecRunner{}.Run(t.Context(), Cmd{Name: name, Args: args, Timeout: 100 * time.Millisecond})
		require.NoError(t, err, "a command that outlived its budget is still a command that ran")
		assert.Less(t, time.Since(start), 2*time.Second,
			"and it must be cut short well before its own five seconds, or nothing is waiting on the budget")
		assert.NotZero(t, res.Code, "a process that was killed did not succeed, and must not be read as having done so")
	})

	t.Run("a command that finishes within its Timeout completes normally", func(t *testing.T) {
		name, args := testproc.Command(t, testproc.Emit, "fast", "", "0")
		res, err := ExecRunner{}.Run(t.Context(), Cmd{Name: name, Args: args, Timeout: 5 * time.Second})
		require.NoError(t, err, "a command that answered in time must succeed")
		assert.Equal(t, "fast", string(res.Stdout),
			"and a budget nobody reached must not change what it said")
	})

	t.Run("Stdin is fed to the program", func(t *testing.T) {
		name, args := testproc.Command(t, testproc.EchoStdin)
		res, err := ExecRunner{}.Run(t.Context(), Cmd{Name: name, Args: args, Stdin: "hunter2\n"})
		require.NoError(t, err, "a command reading its standard input must still run")
		assert.Equal(t, "hunter2\n", string(res.Stdout),
			"a passphrase is handed over on stdin precisely so it never appears in argv, "+
				"and what the program reads there has to be what the caller sent")
	})

	t.Run("Env is added to the inherited environment, not put in its place", func(t *testing.T) {
		name, args := testproc.Command(t, testproc.EchoEnv, "SSHAKKU_TEST_TOKEN", "PATH")
		res, err := ExecRunner{}.Run(t.Context(), Cmd{
			Name: name, Args: args, Env: []string{"SSHAKKU_TEST_TOKEN=set"},
		})
		require.NoError(t, err, "a command given an extra environment entry must still run")
		lines := strings.SplitN(strings.TrimSuffix(string(res.Stdout), "\n"), "\n", 2)
		require.Len(t, lines, 2, "the program has to have answered about both")
		assert.Equal(t, "set", lines[0], "the entry the caller added has to reach the program")
		assert.NotEmpty(t, strings.TrimSpace(lines[1]),
			"and the environment it was added to has to survive: a wallet handed a session token "+
				"still needs the PATH that finds the tools it runs")
	})

	t.Run("a program that cannot be started is an error, not an exit code", func(t *testing.T) {
		res, err := ExecRunner{}.Run(t.Context(), Cmd{Name: "sshakku-no-such-program"})
		require.Error(t, err,
			"a tool that is not installed is not a tool that refused, and a caller deciding whether to "+
				"fall back to another backend has to be able to tell the two apart")
		assert.Zero(t, res.Code,
			"and it must not be given an exit code either, since nothing ever exited")
	})
}

// TestTimeoutDefaults guards the floor under every call site: one that chose no
// budget of its own must still get a finite one. A zero default would restore
// the unbounded wait everywhere at once, and no other test would notice, since
// they all pass a budget of their own.
func TestTimeoutDefaults(t *testing.T) {
	assert.Positive(t, DefaultCommandTimeout,
		"a call site that chose no budget must still get a finite one, or the unbounded wait comes back everywhere at once")
	assert.Positive(t, DefaultInteractiveTimeout, "and so must one that waits on a person")
}
