package secretservice

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// busConfig is the message-bus configuration the private bus is started with,
// as a template: the one service directory it names has to be a fresh one per
// test. secretsServiceFile, dropped into that directory, is what makes
// org.freedesktop.secrets a name the bus knows how to start.
const (
	busConfig          = "testdata/dbus-session.xml"
	secretsServiceFile = "testdata/org.freedesktop.secrets.service"
)

// startSessionBus spawns a private dbus-daemon session bus for the duration of
// the test and points DBUS_SESSION_BUS_ADDRESS at it, so NewClient connects to
// this bus rather than the real desktop session bus. Nothing is running on it
// and it knows how to start nothing. The test is skipped, not failed, when
// dbus-daemon isn't on PATH — these are the only tests in the tree that need a
// real message bus.
func startSessionBus(t *testing.T) {
	t.Helper()
	startBus(t, false)
}

// startActivatableSessionBus spawns the same private bus with a wallet it knows
// how to start but has not started, which is the state a machine is in when a
// Secret Service is installed and nothing has yet asked for a secret.
func startActivatableSessionBus(t *testing.T) {
	t.Helper()
	startBus(t, true)
}

func startBus(t *testing.T, activatable bool) {
	t.Helper()

	bin, err := exec.LookPath("dbus-daemon")
	if err != nil {
		t.Skip("dbus-daemon not found on PATH, skipping Secret Service D-Bus tests")
	}

	// The bus is given a service directory of this test's own and none of the
	// system ones, so which names it can start is something the test states
	// rather than something it inherits from the machine.
	dir := t.TempDir()
	services := filepath.Join(dir, "services")
	require.NoError(t, os.Mkdir(services, 0o700), "create service directory")
	if activatable {
		service, err := os.ReadFile(secretsServiceFile)
		require.NoErrorf(t, err, "read %s", secretsServiceFile)
		installed := filepath.Join(services, filepath.Base(secretsServiceFile))
		require.NoError(t, os.WriteFile(installed, service, 0o600), "install service file")
	}

	template, err := os.ReadFile(busConfig)
	require.NoErrorf(t, err, "read %s", busConfig)
	config := filepath.Join(dir, "bus.xml")
	contents := strings.ReplaceAll(string(template), "@SERVICEDIR@", services)
	require.NoError(t, os.WriteFile(config, []byte(contents), 0o600), "write bus configuration")

	cmd := exec.CommandContext(t.Context(), bin, "--config-file="+config, "--nofork", "--print-address")
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err, "dbus-daemon stdout pipe")
	require.NoError(t, cmd.Start(), "start dbus-daemon")
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	line, err := bufio.NewReader(stdout).ReadString('\n')
	require.NoError(t, err, "read dbus-daemon address")
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", strings.TrimSpace(line))
}
