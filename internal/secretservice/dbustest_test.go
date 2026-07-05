package secretservice

import (
	"bufio"
	"os/exec"
	"strings"
	"testing"
)

// startSessionBus spawns a private dbus-daemon session bus for the duration
// of the test and points DBUS_SESSION_BUS_ADDRESS at it, so NewClient
// connects to this bus rather than the real desktop session bus. The test is
// skipped, not failed, when dbus-daemon isn't on PATH — these are the only
// tests in the tree that need a real message bus.
func startSessionBus(t *testing.T) {
	t.Helper()

	bin, err := exec.LookPath("dbus-daemon")
	if err != nil {
		t.Skip("dbus-daemon not found on PATH, skipping Secret Service D-Bus tests")
	}

	cmd := exec.Command(bin, "--session", "--nofork", "--print-address")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("dbus-daemon stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start dbus-daemon: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatalf("read dbus-daemon address: %v", err)
	}
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", strings.TrimSpace(line))
}
