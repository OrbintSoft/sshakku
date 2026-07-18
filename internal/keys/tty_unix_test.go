//go:build unix

package keys

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// TestReadTTYLineNoTerminalReturnsPromptly confirms that with no controlling
// terminal at all, ReadTTYLine fails immediately with ErrNoTerminal rather
// than blocking — the guarantee the proactive loader and the reactive
// askpass broker both rely on to never hang a headless session. It re-execs
// this test binary detached (Setsid) into its own session, so the child has
// no controlling terminal regardless of what the test runner itself has.
func TestReadTTYLineNoTerminalReturnsPromptly(t *testing.T) {
	if os.Getenv("SSHAKKU_TTY_HELPER") == "1" {
		_, err := ReadTTYLine("prompt", true)
		switch {
		case err == nil:
			os.Exit(1)
		case !errors.Is(err, ErrNoTerminal):
			os.Exit(2)
		default:
			os.Exit(0)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestReadTTYLineNoTerminalReturnsPromptly$")
	cmd.Env = append(os.Environ(), "SSHAKKU_TTY_HELPER=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdin = nil

	start := time.Now()
	err := cmd.Run()
	elapsed := time.Since(start)

	if ctx.Err() != nil {
		t.Fatalf("ReadTTYLine blocked instead of failing immediately with no controlling terminal: %v", ctx.Err())
	}
	if elapsed > 2*time.Second {
		t.Errorf("ReadTTYLine took %v to fail with no controlling terminal, want near-instant", elapsed)
	}
	if err != nil {
		t.Fatalf("helper process did not see ErrNoTerminal as expected: %v", err)
	}
}
