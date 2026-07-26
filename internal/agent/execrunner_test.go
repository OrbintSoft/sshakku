package agent

import (
	"errors"
	"testing"
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
		if err != nil {
			t.Fatalf("Start: %v", err)
		}
		if pid != 4242 {
			t.Errorf("pid = %d, want 4242 from the announced SSH_AGENT_PID", pid)
		}
		if gotName != "/opt/bin/ssh-agent" {
			t.Errorf("bin = %q, want the explicit Path", gotName)
		}
		if len(gotArgs) != 2 || gotArgs[0] != "-a" || gotArgs[1] != "/run/agent.sock" {
			t.Errorf("args = %v, want [-a /run/agent.sock]", gotArgs)
		}
	})

	t.Run("defaults to ssh-agent on PATH when Path is empty", func(t *testing.T) {
		var gotName string
		execOutput = func(name string, args ...string) ([]byte, error) {
			gotName = name
			return []byte("SSH_AGENT_PID=5555;\n"), nil
		}
		pid, err := ExecRunner{}.Start("/run/agent.sock")
		if err != nil {
			t.Fatalf("Start: %v", err)
		}
		if pid != 5555 {
			t.Errorf("pid = %d, want 5555", pid)
		}
		if gotName != "ssh-agent" {
			t.Errorf("bin = %q, want \"ssh-agent\" for PATH resolution", gotName)
		}
	})

	t.Run("wraps an exec failure", func(t *testing.T) {
		execOutput = func(string, ...string) ([]byte, error) {
			return nil, errors.New("cannot execute")
		}
		if _, err := (ExecRunner{}).Start("/run/agent.sock"); err == nil {
			t.Fatal("want an error when ssh-agent cannot be executed")
		}
	})
}
