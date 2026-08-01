//go:build linux

package main

import (
	"strings"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/paths"
)

// TestDoctorReportOnAnUnconfiguredMachine verifies F25 and F26 for the Linux
// half: a machine nobody has configured is told it uses the freedesktop Secret
// Service, and asked for the session bus that wallet is reached over.
//
// The settings come from the configuration layer with no file present, because
// that is the case being described — writing the backend in here would ask a
// question the user never asks.
func TestDoctorReportOnAnUnconfiguredMachine(t *testing.T) {
	layout := paths.Layout{ConfigDir: t.TempDir()}
	settings := loadSettings(layout, "test", discardLogger{})

	view := walletView(settings, probeWith("linux", nil, nil, "unix:path=/run/bus", nil))

	if view.Backend != config.SecretBackendSecretService {
		t.Errorf("doctor names %q as the wallet, want %q", view.Backend, config.SecretBackendSecretService)
	}
	req := requirement(t, view, "session bus")
	if !req.Present || req.Detail != "unix:path=/run/bus" {
		t.Errorf("session bus = %+v, want the address the shell has", req)
	}
}

// TestDoctorReportsAMissingSessionBus covers the other answer: the wallet is
// there to be named, but the bus it is reached over is not, which is a piece
// the user can go and provide — unlike a wallet the system has not got.
func TestDoctorReportsAMissingSessionBus(t *testing.T) {
	settings := config.Settings{SecretBackend: config.SecretBackendSecretService}

	view := walletView(settings, probeWith("linux", nil, nil, "", nil))

	req := requirement(t, view, "session bus")
	if req.Present {
		t.Error("a session bus must not be reported as present when the address is unset")
	}
	if !strings.Contains(req.Detail, "DBUS_SESSION_BUS_ADDRESS is unset") {
		t.Errorf("detail = %q, want it to name what is unset", req.Detail)
	}
}

// TestKeePassXCOverTheSecretServiceOnLinux is the route rather than the wallet:
// reaching KeePassXC through the Secret Service is reaching it over the session
// bus, exactly as any other wallet behind that API.
func TestKeePassXCOverTheSecretServiceOnLinux(t *testing.T) {
	settings := config.Settings{
		SecretBackend:  config.SecretBackendKeePassXC,
		KeePassXCRoute: config.KeePassXCRouteAuto,
	}

	view := walletView(settings, probeWith("linux", nil, nil, "unix:path=/run/bus", nil))

	if view.Route != config.KeePassXCRouteSecretService {
		t.Errorf("route = %q, want the secret service, which is what Linux picks", view.Route)
	}
	req := requirement(t, view, "session bus")
	if !req.Present || req.Detail != "unix:path=/run/bus" {
		t.Errorf("session bus = %+v, want the address the shell has", req)
	}
}

// TestPlatformWalletViewNamesWhateverItIsGiven covers the fallback beside the
// Secret Service: a name the configuration layer would never produce here still
// gets named back, with nothing claimed about it, rather than being reported as
// some other wallet. Saying "this is what you asked for" remains an answer to
// "which wallet would be used"; inventing a requirement for it would not be.
func TestPlatformWalletViewNamesWhateverItIsGiven(t *testing.T) {
	view := probeWith("linux", nil, nil, "", nil).platformWalletView("something-else")

	if view.Backend != "something-else" {
		t.Errorf("backend = %q, want the name it was given back", view.Backend)
	}
	if len(view.Requirements) != 0 {
		t.Errorf("requirements = %+v, want none for a wallet nothing is known about", view.Requirements)
	}
}

// discardLogger is the session log a test has no use for.
type discardLogger struct{}

func (discardLogger) Log(string, string) error { return nil }
