package keys

import (
	"strings"
	"testing"
	"time"
)

func TestExecRunnerRun(t *testing.T) {
	t.Run("captures stdout, stderr, and exit code", func(t *testing.T) {
		res, err := ExecRunner{}.Run(Cmd{Name: "sh", Args: []string{"-c", "echo out; echo err >&2; exit 3"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.TrimSpace(string(res.Stdout)) != "out" {
			t.Fatalf("stdout = %q, want %q", res.Stdout, "out")
		}
		if strings.TrimSpace(string(res.Stderr)) != "err" {
			t.Fatalf("stderr = %q, want %q", res.Stderr, "err")
		}
		if res.Code != 3 {
			t.Fatalf("code = %d, want 3", res.Code)
		}
	})

	t.Run("zero Timeout does not bound the command", func(t *testing.T) {
		res, err := ExecRunner{}.Run(Cmd{Name: "sh", Args: []string{"-c", "sleep 0.2; echo done"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.TrimSpace(string(res.Stdout)) != "done" {
			t.Fatalf("stdout = %q, want %q", res.Stdout, "done")
		}
	})

	t.Run("a positive Timeout kills a command that outlives it", func(t *testing.T) {
		start := time.Now()
		res, err := ExecRunner{}.Run(Cmd{Name: "sh", Args: []string{"-c", "sleep 5"}, Timeout: 100 * time.Millisecond})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Fatalf("Run took %s, want well under the 5s sleep — Timeout did not bound it", elapsed)
		}
		if res.Code == 0 {
			t.Fatalf("code = 0, want a non-zero (signaled) exit for a killed process")
		}
	})

	t.Run("a command that finishes within its Timeout completes normally", func(t *testing.T) {
		res, err := ExecRunner{}.Run(Cmd{Name: "sh", Args: []string{"-c", "echo fast"}, Timeout: 5 * time.Second})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.TrimSpace(string(res.Stdout)) != "fast" {
			t.Fatalf("stdout = %q, want %q", res.Stdout, "fast")
		}
	})
}
