package cli

import (
	"os"
	"testing"
)

// TestMain unsets what would let a command built here reach a daemon or a
// screen the machine happens to have: a wallet is opened over the session bus
// and a dialog is raised on a display, and both addresses are read from the
// environment.
//
// The commands under test are assembled from realDeps, so the wallet seam is
// the real one; only what it is pointed at is decided here. Left set, these
// tests do not run the same way twice — they ask whatever wallet is listening
// to do as they say, and pass or hang on its answer. `doctor --fix` against a
// KeePassXC holding org.freedesktop.secrets raises a dialog nobody is there to
// answer, and the test spends the whole interactive budget before failing.
//
// Unset once for the package rather than per helper, because it must hold for
// every test here and not only the ones that remembered to ask. A test that
// wants a real daemon belongs with the integration suites, which opt in.
func TestMain(m *testing.M) {
	for _, name := range []string{"DBUS_SESSION_BUS_ADDRESS", "DISPLAY", "WAYLAND_DISPLAY"} {
		_ = os.Unsetenv(name)
	}
	os.Exit(m.Run())
}
