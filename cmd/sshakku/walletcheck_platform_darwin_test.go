//go:build darwin

package main

import (
	"runtime"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/paths"
	"github.com/stretchr/testify/assert"
)

// TestDoctorReportOnAnUnconfiguredMachine is the defect as a user met it: on a
// Mac with nothing configured, `sshakku doctor` named the freedesktop Secret
// Service and reported DBUS_SESSION_BUS_ADDRESS missing, while the passphrases
// were going into the keychain all along.
//
// It verifies F25 and F26 together: the report names the wallet that would
// actually be used, and never asks for a piece of a wallet this system has not
// got. The settings come from the configuration layer with no file present,
// because that is the case — a machine nobody has configured — and writing the
// backend in here would ask a question the user never asked.
func TestDoctorReportOnAnUnconfiguredMachine(t *testing.T) {
	layout := paths.Layout{ConfigDir: t.TempDir()}
	settings := loadSettings(layout, "test", discardLogger{})

	view := walletView(t.Context(), settings, probeWith(runtime.GOOS, nil, nil, "", nil))

	assert.Equal(t, config.SecretBackendKeychain, view.Backend,
		"the report must name the wallet the passphrases actually go into")
	for _, req := range view.Requirements {
		assert.NotEqualf(t, "session bus", req.Name,
			"%s has no D-Bus session bus, so asking for one sends the user after a piece that cannot exist (%q)",
			runtime.GOOS, req.Detail)
	}
}

// TestKeePassXCOverASecretServiceThisPlatformHasNot is the route rather than
// the wallet. Unlike the wallet name, a route the user pinned is answered under
// its own name instead of being swapped for another (F23) — so it is described
// here too, but the answer is that this way in does not exist on this system,
// not that a piece of it is missing.
func TestKeePassXCOverASecretServiceThisPlatformHasNot(t *testing.T) {
	settings := config.Settings{
		SecretBackend:  config.SecretBackendKeePassXC,
		KeePassXCRoute: config.KeePassXCRouteSecretService,
	}

	view := walletView(t.Context(), settings, probeWith(runtime.GOOS, nil, nil, "", nil))

	assert.Equal(t, config.KeePassXCRouteSecretService, view.Route,
		"a route the user pinned is answered under its own name, not swapped for another")
	req := requirement(t, view, "secret service")
	assert.False(t, req.Present, "a Secret Service must not be reported as present on a system that has none")
	assert.Contains(t, req.Detail, "provides no freedesktop Secret Service",
		"and the answer must be that the way in does not exist, not that a piece of it is missing")
	assert.Contains(t, req.Detail, runtime.GOOS, "naming the platform that has not got it")
}

// discardLogger is the session log a test has no use for.
type discardLogger struct{}

func (discardLogger) Log(string, string) error { return nil }
