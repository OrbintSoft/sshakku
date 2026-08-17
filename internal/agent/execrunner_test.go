package agent

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/testproc"
)

func TestExecRunnerStart(t *testing.T) {
	orig := execOutput
	t.Cleanup(func() { execOutput = orig })

	t.Run("uses the explicit path and parses the announced pid", func(t *testing.T) {
		var gotName string
		var gotArgs []string
		execOutput = func(_ context.Context, name string, args ...string) ([]byte, error) {
			gotName, gotArgs = name, args
			return []byte("SSH_AUTH_SOCK=/tmp/a.sock; export SSH_AUTH_SOCK;\nSSH_AGENT_PID=4242; export SSH_AGENT_PID;\n"), nil
		}
		pid, err := ExecRunner{Path: "/opt/bin/ssh-agent"}.Start(t.Context(), "/run/agent.sock")
		require.NoError(t, err, "Start")
		assert.Equal(t, 4242, pid, "pid must come from the announced SSH_AGENT_PID")
		assert.Equal(t, "/opt/bin/ssh-agent", gotName, "the explicit Path must be used")
		assert.Equal(t, []string{"-a", "/run/agent.sock"}, gotArgs, "args")
	})

	t.Run("defaults to ssh-agent on PATH when Path is empty", func(t *testing.T) {
		var gotName string
		execOutput = func(_ context.Context, name string, args ...string) ([]byte, error) {
			gotName = name
			return []byte("SSH_AGENT_PID=5555;\n"), nil
		}
		pid, err := ExecRunner{}.Start(t.Context(), "/run/agent.sock")
		require.NoError(t, err, "Start")
		assert.Equal(t, 5555, pid, "pid")
		assert.Equal(t, "ssh-agent", gotName, "a bare name is what leaves the lookup to PATH")
	})

	t.Run("wraps an exec failure", func(t *testing.T) {
		execOutput = func(context.Context, string, ...string) ([]byte, error) {
			return nil, errors.New("cannot execute")
		}
		_, err := (ExecRunner{}).Start(t.Context(), "/run/agent.sock")
		assert.Error(t, err, "an ssh-agent that cannot be executed must be reported")
	})

	// An agent that refuses to start says why on its stderr, and an exit
	// status on its own says nothing anybody can act on. What the agent said
	// has to reach whoever is reading.
	// These two need a process that really exits non-zero, because what is
	// under test is what os/exec puts in an ExitError and what Start makes of
	// it. The process is this test binary re-entered — see internal/testproc.
	t.Run("reports what the agent itself said", func(t *testing.T) {
		name, args := testproc.Command(t, testproc.Emit, "", "bind: Invalid argument", "1")
		execOutput = func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).Output()
		}
		_, err := (ExecRunner{}).Start(t.Context(), "/run/agent.sock")
		require.Error(t, err, "an ssh-agent that exits non-zero must be reported")
		assert.ErrorContains(t, err, "bind: Invalid argument", "the error must carry what ssh-agent said")
	})

	// Nothing to add is not an excuse to lose the exit status.
	t.Run("an exit with nothing said is still reported", func(t *testing.T) {
		name, args := testproc.Command(t, testproc.Emit, "", "", "3")
		execOutput = func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).Output()
		}
		_, err := (ExecRunner{}).Start(t.Context(), "/run/agent.sock")
		require.Error(t, err, "an ssh-agent that exits non-zero in silence must still be reported")

		var exit *exec.ExitError
		require.ErrorAs(t, err, &exit,
			"and reported as what it was: a program that ran and refused, not one that could not be started")
		assert.Equal(t, 3, exit.ExitCode(), "the status is the only thing it said, so it is the one thing that must survive")
	})
}
