package agent

import (
	"errors"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecRunnerStart(t *testing.T) {
	orig := execOutput
	t.Cleanup(func() { execOutput = orig })

	t.Run("uses the explicit path and parses the announced pid", func(t *testing.T) {
		var gotName string
		var gotArgs []string
		execOutput = func(name string, args ...string) ([]byte, error) {
			gotName, gotArgs = name, args
			return []byte("SSH_AUTH_SOCK=/tmp/a.sock; export SSH_AUTH_SOCK;\nSSH_AGENT_PID=4242; export SSH_AGENT_PID;\n"), nil
		}
		pid, err := ExecRunner{Path: "/opt/bin/ssh-agent"}.Start("/run/agent.sock")
		require.NoError(t, err, "Start")
		assert.Equal(t, 4242, pid, "pid must come from the announced SSH_AGENT_PID")
		assert.Equal(t, "/opt/bin/ssh-agent", gotName, "the explicit Path must be used")
		assert.Equal(t, []string{"-a", "/run/agent.sock"}, gotArgs, "args")
	})

	t.Run("defaults to ssh-agent on PATH when Path is empty", func(t *testing.T) {
		var gotName string
		execOutput = func(name string, args ...string) ([]byte, error) {
			gotName = name
			return []byte("SSH_AGENT_PID=5555;\n"), nil
		}
		pid, err := ExecRunner{}.Start("/run/agent.sock")
		require.NoError(t, err, "Start")
		assert.Equal(t, 5555, pid, "pid")
		assert.Equal(t, "ssh-agent", gotName, "a bare name is what leaves the lookup to PATH")
	})

	t.Run("wraps an exec failure", func(t *testing.T) {
		execOutput = func(string, ...string) ([]byte, error) {
			return nil, errors.New("cannot execute")
		}
		_, err := (ExecRunner{}).Start("/run/agent.sock")
		assert.Error(t, err, "an ssh-agent that cannot be executed must be reported")
	})

	// An agent that refuses to start says why on its stderr, and an exit
	// status on its own says nothing anybody can act on. What the agent said
	// has to reach whoever is reading.
	t.Run("reports what the agent itself said", func(t *testing.T) {
		execOutput = func(string, ...string) ([]byte, error) {
			return exec.Command("sh", "-c", "echo 'bind: Invalid argument' >&2; exit 1").Output()
		}
		_, err := (ExecRunner{}).Start("/run/agent.sock")
		require.Error(t, err, "an ssh-agent that exits non-zero must be reported")
		assert.ErrorContains(t, err, "bind: Invalid argument", "the error must carry what ssh-agent said")
	})

	// Nothing to add is not an excuse to lose the exit status.
	t.Run("an exit with nothing said is still reported", func(t *testing.T) {
		execOutput = func(string, ...string) ([]byte, error) {
			return exec.Command("sh", "-c", "exit 3").Output()
		}
		_, err := (ExecRunner{}).Start("/run/agent.sock")
		assert.Error(t, err, "an ssh-agent that exits non-zero in silence must still be reported")
	})
}
