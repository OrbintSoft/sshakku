//go:build darwin

package main

import (
	"runtime"
	"strings"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/paths"
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

	view := walletView(settings, probeWith(runtime.GOOS, nil, nil, "", nil))

	if view.Backend != config.SecretBackendKeychain {
		t.Errorf("doctor names %q as the wallet, want %q — the report names one SSHakku would not open",
			view.Backend, config.SecretBackendKeychain)
	}
	for _, req := range view.Requirements {
		if req.Name == "session bus" {
			t.Errorf("doctor asks for a D-Bus session bus on %s, which has none: %q",
				runtime.GOOS, req.Detail)
		}
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

	view := walletView(settings, probeWith(runtime.GOOS, nil, nil, "", nil))

	if view.Route != config.KeePassXCRouteSecretService {
		t.Errorf("route = %q, want the pinned one to be answered under its own name", view.Route)
	}
	req := requirement(t, view, "secret service")
	if req.Present {
		t.Error("a Secret Service must not be reported as present on a system that has none")
	}
	if !strings.Contains(req.Detail, "provides no freedesktop Secret Service") {
		t.Errorf("detail = %q, want it to say the platform has no such API", req.Detail)
	}
	if !strings.Contains(req.Detail, runtime.GOOS) {
		t.Errorf("detail = %q, want it to name the platform", req.Detail)
	}
}

// discardLogger is the session log a test has no use for.
type discardLogger struct{}

func (discardLogger) Log(string, string) error { return nil }
