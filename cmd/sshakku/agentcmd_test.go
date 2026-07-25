package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/agent"
	"github.com/OrbintSoft/sshakku/internal/paths"
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
		if got := d.shellInit(&out, &errOut); got != 0 {
			t.Fatalf("shellInit = %d, want 0; stderr=%q", got, errOut.String())
		}
		s := out.String()
		for _, want := range []string{"agent_sock='/run/sshakku/agent.sock'", "agent_lock=", "log_file="} {
			if !strings.Contains(s, want) {
				t.Errorf("output %q missing %q", s, want)
			}
		}
	})

	t.Run("ensure failure propagates the exit code", func(t *testing.T) {
		tempRuntimeEnv(t)
		d := depsWithEnsurer(fakeEnsurer{err: errors.New("boom")})
		var out, errOut bytes.Buffer
		if got := d.shellInit(&out, &errOut); got != 1 {
			t.Fatalf("shellInit (ensure fails) = %d, want 1", got)
		}
		if out.Len() != 0 {
			t.Errorf("stdout = %q, want no assignments when ensure fails", out.String())
		}
	})

	t.Run("uncreatable layout returns 1", func(t *testing.T) {
		home := tempRuntimeEnv(t)
		// A plain file where ~/.config should be a directory makes paths.Ensure
		// fail to create the config dir.
		if err := os.WriteFile(filepath.Join(home, ".config"), []byte("not a dir"), 0o600); err != nil {
			t.Fatalf("seed ~/.config file: %v", err)
		}
		d := depsWithEnsurer(fakeEnsurer{})
		var out, errOut bytes.Buffer
		if got := d.shellInit(&out, &errOut); got != 1 {
			t.Fatalf("shellInit (uncreatable layout) = %d, want 1", got)
		}
		if !strings.Contains(errOut.String(), "sshakku:") {
			t.Errorf("stderr = %q, want a sshakku: error line", errOut.String())
		}
	})

	t.Run("stdout write error returns 1", func(t *testing.T) {
		tempRuntimeEnv(t)
		d := depsWithEnsurer(fakeEnsurer{res: agent.EnsureResult{LiveSock: "/run/sshakku/agent.sock"}})
		var errOut bytes.Buffer
		if got := d.shellInit(errWriter{}, &errOut); got != 1 {
			t.Errorf("shellInit (failing stdout) = %d, want 1", got)
		}
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
		if got := d.ensureAgent(&out, &errOut); got != 0 {
			t.Fatalf("ensureAgent = %d, want 0; stderr=%q", got, errOut.String())
		}
		if want := "agent_sock='/run/sshakku/agent.sock'\n"; out.String() != want {
			t.Errorf("output = %q, want %q", out.String(), want)
		}
	})

	t.Run("ensure failure propagates the exit code", func(t *testing.T) {
		tempRuntimeEnv(t)
		d := depsWithEnsurer(fakeEnsurer{err: errors.New("boom")})
		if got := d.ensureAgent(io.Discard, io.Discard); got != 1 {
			t.Errorf("ensureAgent (ensure fails) = %d, want 1", got)
		}
	})

	t.Run("uncreatable layout returns 1", func(t *testing.T) {
		home := tempRuntimeEnv(t)
		if err := os.WriteFile(filepath.Join(home, ".config"), []byte("not a dir"), 0o600); err != nil {
			t.Fatalf("seed ~/.config file: %v", err)
		}
		if got := depsWithEnsurer(fakeEnsurer{}).ensureAgent(io.Discard, io.Discard); got != 1 {
			t.Errorf("ensureAgent (uncreatable layout) = %d, want 1", got)
		}
	})

	t.Run("stdout write error returns 1", func(t *testing.T) {
		tempRuntimeEnv(t)
		d := depsWithEnsurer(fakeEnsurer{res: agent.EnsureResult{LiveSock: "/run/sshakku/agent.sock"}})
		if got := d.ensureAgent(errWriter{}, io.Discard); got != 1 {
			t.Errorf("ensureAgent (failing stdout) = %d, want 1", got)
		}
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
		if sock != "/run/sshakku/agent.sock" || code != 0 {
			t.Fatalf("runEnsure = (%q, %d), want (the live socket, 0)", sock, code)
		}
		if errOut.Len() != 0 {
			t.Errorf("stderr = %q, want nothing on a clean run", errOut.String())
		}
	})

	t.Run("anomaly is reported but still succeeds", func(t *testing.T) {
		d := depsWithEnsurer(fakeEnsurer{res: agent.EnsureResult{LiveSock: "/run/sshakku/agent.sock", Anomaly: "adopted a foreign agent"}})
		var errOut bytes.Buffer
		sock, code := d.runEnsure(&errOut, env, layout)
		if sock != "/run/sshakku/agent.sock" || code != 0 {
			t.Fatalf("runEnsure = (%q, %d), want (the live socket, 0)", sock, code)
		}
		if !strings.Contains(errOut.String(), "adopted a foreign agent") {
			t.Errorf("stderr = %q, want the anomaly reported", errOut.String())
		}
	})

	t.Run("ensure error returns 1", func(t *testing.T) {
		d := depsWithEnsurer(fakeEnsurer{err: errors.New("boom")})
		var errOut bytes.Buffer
		sock, code := d.runEnsure(&errOut, env, layout)
		if sock != "" || code != 1 {
			t.Fatalf("runEnsure = (%q, %d), want (\"\", 1)", sock, code)
		}
		if !strings.Contains(errOut.String(), "boom") {
			t.Errorf("stderr = %q, want the error reported", errOut.String())
		}
	})
}
