package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// fakeAgentEnv gates the self-exec fake ssh-agent: when set to a pid string, the
// test binary impersonates ssh-agent instead of running the suite.
const fakeAgentEnv = "SSHAKKU_FAKE_SSH_AGENT"

// TestMain lets the test binary double as a fake `ssh-agent`. ExecRunner.Start
// re-execs whatever binary it is pointed at, so a child invocation with
// SSHAKKU_FAKE_SSH_AGENT set prints the environment a real ssh-agent would
// announce and exits, letting Start be covered without a real ssh-agent present.
func TestMain(m *testing.M) {
	if pid := os.Getenv(fakeAgentEnv); pid != "" {
		fmt.Printf("SSH_AUTH_SOCK=/tmp/fake.sock; export SSH_AUTH_SOCK;\nSSH_AGENT_PID=%s; export SSH_AGENT_PID;\n", pid)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestExecRunnerStart(t *testing.T) {
	socket := filepath.Join(shortDir(t), "agent.sock")

	t.Run("explicit path", func(t *testing.T) {
		t.Setenv(fakeAgentEnv, "4242") // inherited by the re-exec'd child
		pid, err := ExecRunner{Path: os.Args[0]}.Start(socket)
		if err != nil {
			t.Fatalf("Start: %v", err)
		}
		if pid != 4242 {
			t.Errorf("pid = %d, want 4242 from the fake agent", pid)
		}
	})

	t.Run("resolves ssh-agent on PATH", func(t *testing.T) {
		bindir := shortDir(t)
		if err := os.Symlink(os.Args[0], filepath.Join(bindir, "ssh-agent")); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", bindir)
		t.Setenv(fakeAgentEnv, "5555")
		pid, err := ExecRunner{}.Start(socket) // empty Path → resolves "ssh-agent"
		if err != nil {
			t.Fatalf("Start: %v", err)
		}
		if pid != 5555 {
			t.Errorf("pid = %d, want 5555 from the PATH-resolved fake agent", pid)
		}
	})

	t.Run("exec fails", func(t *testing.T) {
		_, err := ExecRunner{Path: filepath.Join(shortDir(t), "nonexistent")}.Start(socket)
		if err == nil {
			t.Fatal("want an error when the ssh-agent binary cannot be executed")
		}
	})
}
