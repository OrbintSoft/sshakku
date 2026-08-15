package backend

import (
	"os"
	"testing"
)

// TestMain unsets the session bus and display addresses for this package's
// tests. Open returns the wallet the settings select, and for the routes that
// go through the freedesktop Secret Service that means connecting to whatever
// bus the environment names — so a test asserting which wallet was chosen
// would otherwise be waiting on a daemon it never meant to talk to.
//
// A test that wants a real daemon belongs with the integration suites, which
// opt in.
func TestMain(m *testing.M) {
	for _, name := range []string{"DBUS_SESSION_BUS_ADDRESS", "DISPLAY", "WAYLAND_DISPLAY"} {
		_ = os.Unsetenv(name)
	}
	os.Exit(m.Run())
}
