//go:build unix

package agent

import (
	"os/exec"
	"testing"
)

// TestSysSignalerTerminate covers SysSignaler.Terminate against a real child
// process we own: it spawns a long sleep, delivers SIGTERM, and confirms the
// child exits from the signal. The process is fully self-contained and reaped
// here, so the test leaves nothing behind.
func TestSysSignalerTerminate(t *testing.T) {
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sleep: %v", err)
	}

	if err := (SysSignaler{}).Terminate(cmd.Process.Pid); err != nil {
		t.Fatalf("Terminate(%d): %v", cmd.Process.Pid, err)
	}

	// The child must have been terminated by the signal, not exited normally.
	err := cmd.Wait()
	if err == nil {
		t.Fatal("sleep exited cleanly, want termination by SIGTERM")
	}
}
