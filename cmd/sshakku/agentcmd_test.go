package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/agent"
	"github.com/OrbintSoft/sshakku/internal/paths"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeEnsurer stands in for agent.Manager so shell-init/ensure-agent and
// runEnsure run their result-handling logic without spawning, reaping, or
// adopting a real ssh-agent on the test host.
type fakeEnsurer struct {
	res agent.EnsureResult
	err error
}

func (f fakeEnsurer) EnsureAgent(agent.EnsureConfig, agent.Logger) (agent.EnsureResult, error) {
	return f.res, f.err
}

// errWriter fails every write, so a command's output-write error branch runs.
type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

// tempRuntimeEnv points HOME and the XDG dirs at fresh temp dirs so paths.Resolve
// and paths.Ensure build and create the runtime layout entirely off the real
// state, runtime, and config dirs.
func tempRuntimeEnv(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	return home
}

func depsWithEnsurer(e agentEnsurer) deps {
	d := realDeps()
	d.ensurer = e
	return d
}

// TestShellInit covers shell-init end to end against a fake ensurer: a healthy
// agent prints the three shell assignments, a failed ensure propagates its exit
// code, an uncreatable runtime layout returns 1, and a failing stdout surfaces
// as a write error.
func TestShellInit(t *testing.T) {
	t.Run("healthy agent prints the shell assignments", func(t *testing.T) {
		tempRuntimeEnv(t)
		d := depsWithEnsurer(fakeEnsurer{res: agent.EnsureResult{LiveSock: "/run/sshakku/agent.sock"}})
		var out, errOut bytes.Buffer
		require.Zerof(t, d.shellInit(&out, &errOut), "shellInit must succeed; stderr=%q", errOut.String())
		// Three assignments the shell needs; assert, so one run names every one
		// that is missing rather than only the first.
		assert.Contains(t, out.String(), "agent_sock='/run/sshakku/agent.sock'", "the socket the shell must talk to")
		assert.Contains(t, out.String(), "agent_lock=", "the lock the shell must take")
		assert.Contains(t, out.String(), "log_file=", "the log the shell must write to")
	})

	t.Run("ensure failure propagates the exit code", func(t *testing.T) {
		tempRuntimeEnv(t)
		d := depsWithEnsurer(fakeEnsurer{err: errors.New("boom")})
		var out, errOut bytes.Buffer
		assert.Equal(t, 1, d.shellInit(&out, &errOut), "an agent that could not be ensured is a failed init")
		assert.Empty(t, out.String(),
			"a shell must not be given assignments pointing at an agent that is not there")
	})

	t.Run("uncreatable layout returns 1", func(t *testing.T) {
		home := tempRuntimeEnv(t)
		// A plain file where ~/.config should be a directory makes paths.Ensure
		// fail to create the config dir.
		require.NoError(t, os.WriteFile(filepath.Join(home, ".config"), []byte("not a dir"), 0o600),
			"seed a file where the config directory should be")
		d := depsWithEnsurer(fakeEnsurer{})
		var out, errOut bytes.Buffer
		assert.Equal(t, 1, d.shellInit(&out, &errOut), "a layout that could not be created is a failed init")
		assert.Contains(t, errOut.String(), "sshakku:", "the reason must reach the user, named")
	})

	t.Run("stdout write error returns 1", func(t *testing.T) {
		tempRuntimeEnv(t)
		d := depsWithEnsurer(fakeEnsurer{res: agent.EnsureResult{LiveSock: "/run/sshakku/agent.sock"}})
		var errOut bytes.Buffer
		assert.Equal(t, 1, d.shellInit(errWriter{}, &errOut),
			"assignments the shell never received must not be reported as delivered")
	})
}

// TestEnsureAgent covers the ensure-agent command against a fake ensurer: a
// healthy agent prints the single agent_sock assignment, a failed ensure
// propagates its code, an uncreatable layout returns 1, and a failing stdout
// surfaces as a write error.
func TestEnsureAgent(t *testing.T) {
	t.Run("healthy agent prints agent_sock", func(t *testing.T) {
		tempRuntimeEnv(t)
		d := depsWithEnsurer(fakeEnsurer{res: agent.EnsureResult{LiveSock: "/run/sshakku/agent.sock"}})
		var out, errOut bytes.Buffer
		require.Zerof(t, d.ensureAgent(&out, &errOut), "ensureAgent must succeed; stderr=%q", errOut.String())
		assert.Equal(t, "agent_sock='/run/sshakku/agent.sock'\n", out.String(),
			"this command prints the socket and nothing else")
	})

	t.Run("ensure failure propagates the exit code", func(t *testing.T) {
		tempRuntimeEnv(t)
		d := depsWithEnsurer(fakeEnsurer{err: errors.New("boom")})
		assert.Equal(t, 1, d.ensureAgent(io.Discard, io.Discard),
			"an agent that could not be ensured must be reported as such")
	})

	t.Run("uncreatable layout returns 1", func(t *testing.T) {
		home := tempRuntimeEnv(t)
		require.NoError(t, os.WriteFile(filepath.Join(home, ".config"), []byte("not a dir"), 0o600),
			"seed a file where the config directory should be")
		assert.Equal(t, 1, depsWithEnsurer(fakeEnsurer{}).ensureAgent(io.Discard, io.Discard),
			"a layout that could not be created must be reported as such")
	})

	t.Run("stdout write error returns 1", func(t *testing.T) {
		tempRuntimeEnv(t)
		d := depsWithEnsurer(fakeEnsurer{res: agent.EnsureResult{LiveSock: "/run/sshakku/agent.sock"}})
		assert.Equal(t, 1, d.ensureAgent(errWriter{}, io.Discard),
			"a socket the caller never received must not be reported as delivered")
	})
}

// TestRunEnsure covers runEnsure's result handling directly: a healthy result
// returns the live socket with code 0, an adopted-foreign anomaly is reported to
// stderr while still succeeding, and an EnsureAgent error logs and returns 1.
func TestRunEnsure(t *testing.T) {
	layout := paths.Layout{
		AgentSock: "/run/sshakku/agent.sock",
		AgentLock: "/run/sshakku/.start.lock",
		LogFile:   filepath.Join(t.TempDir(), "sessions.log"),
	}
	env := paths.Env{Home: t.TempDir(), UID: 1000}

	t.Run("healthy returns the live socket", func(t *testing.T) {
		d := depsWithEnsurer(fakeEnsurer{res: agent.EnsureResult{LiveSock: "/run/sshakku/agent.sock"}})
		var errOut bytes.Buffer
		sock, code := d.runEnsure(&errOut, env, layout)
		assert.Zero(t, code, "a healthy agent is not a failure")
		assert.Equal(t, "/run/sshakku/agent.sock", sock, "the socket the agent is live on")
		assert.Empty(t, errOut.String(), "a clean run has nothing to say")
	})

	t.Run("anomaly is reported but still succeeds", func(t *testing.T) {
		d := depsWithEnsurer(fakeEnsurer{res: agent.EnsureResult{LiveSock: "/run/sshakku/agent.sock", Anomaly: "adopted a foreign agent"}})
		var errOut bytes.Buffer
		sock, code := d.runEnsure(&errOut, env, layout)
		assert.Zero(t, code, "an anomaly worth mentioning is not a reason to fail the login")
		assert.Equal(t, "/run/sshakku/agent.sock", sock, "the socket the agent is live on")
		assert.Contains(t, errOut.String(), "adopted a foreign agent", "but it must still be said out loud")
	})

	t.Run("ensure error returns 1", func(t *testing.T) {
		d := depsWithEnsurer(fakeEnsurer{err: errors.New("boom")})
		var errOut bytes.Buffer
		sock, code := d.runEnsure(&errOut, env, layout)
		assert.Equal(t, 1, code, "an agent that could not be ensured is a failure")
		assert.Empty(t, sock, "and no socket may be handed back for one")
		assert.Contains(t, errOut.String(), "boom", "the reason must reach the user")
	})
}
