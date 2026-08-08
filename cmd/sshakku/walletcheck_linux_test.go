//go:build linux

package main

import (
	"strings"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/diagnose"
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

// TestDoctorReportsAWalletThatCanHoldNothingHere verifies F25 end of the wiring:
// on a machine with no screen the compartment cannot be created, so no
// passphrase can ever be saved — and that has to reach the findings, where a
// user looks for what is wrong, rather than staying a detail of the wallet
// section.
func TestDoctorReportsAWalletThatCanHoldNothingHere(t *testing.T) {
	settings := config.Settings{SecretBackend: config.SecretBackendSecretService}
	probe := probeWith("linux", nil, nil, "unix:path=/run/bus", nil).
		withLook(secretServiceLook{running: true}, false)

	view := walletView(settings, probe)

	req := requirement(t, view, "compartment")
	if req.Present || req.Undetermined {
		t.Errorf("compartment = %+v, want a piece that is missing", req)
	}
	findings := diagnose.WalletFindings(view)
	if len(findings) != 1 || !strings.Contains(findings[0], "compartment") {
		t.Errorf("findings = %q, want one naming the compartment", findings)
	}
}

// TestDoctorReportsTheCompartmentTheSettingsName covers the other half of that:
// the compartment described is the one entries would go into, so a user who
// named their own is told about theirs.
func TestDoctorReportsTheCompartmentTheSettingsName(t *testing.T) {
	settings := config.Settings{
		SecretBackend:   config.SecretBackendSecretService,
		SecretContainer: "my-own",
	}
	probe := probeWith("linux", nil, nil, "unix:path=/run/bus", nil).
		withLook(secretServiceLook{running: true}, false)

	req := requirement(t, walletView(settings, probe), "compartment")

	if !strings.Contains(req.Detail, "my-own") {
		t.Errorf("compartment detail = %q, want it to name the configured compartment", req.Detail)
	}
}

// TestKeePassXCOverTheSecretServiceSeesAnEmptyBus is the route rather than the
// wallet: reaching KeePassXC this way needs something answering on the bus just
// as any other wallet behind that API does, and a bus with nothing on it is a
// piece the user can go and provide.
func TestKeePassXCOverTheSecretServiceSeesAnEmptyBus(t *testing.T) {
	settings := config.Settings{
		SecretBackend:  config.SecretBackendKeePassXC,
		KeePassXCRoute: config.KeePassXCRouteSecretService,
	}
	probe := probeWith("linux", nil, nil, "unix:path=/run/bus", nil).
		withLook(secretServiceLook{}, false)

	view := walletView(settings, probe)

	if req := requirement(t, view, "secret service"); req.Present {
		t.Errorf("secret service = %+v, want it reported missing on a bus with nothing on it", req)
	}
	for _, req := range view.Requirements {
		if req.Name == "compartment" {
			t.Error("this route's entries live in a database the user opened; the desktop has no compartment to describe")
		}
	}
}

// TestALookThatCouldNotBeTakenSaysSo verifies F41 at the seam where the report
// meets the bus: a look that fails is not an answer. Reporting it as one would
// have the report state, as fact, that a wallet is not there — on the strength
// of never having managed to ask.
func TestALookThatCouldNotBeTakenSaysSo(t *testing.T) {
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/nonexistent/bus")

	look := realSecretServiceLook("sshakku", "sshakku")

	if !look.lookFailed {
		t.Error("a look that never reached a bus must say it failed")
	}
	if req := serviceRequirement(look); !req.Undetermined || req.Present {
		t.Errorf("secret service = %+v, want it undetermined rather than declared absent", req)
	}
}

// TestMakingACompartmentWithNoWalletToMakeItIn verifies F42 where it is easiest
// to get wrong: a repair that could not be performed has to say so. A maker that
// swallowed the failure would have --fix report a compartment it never made, and
// the next login would be the one to find out.
func TestMakingACompartmentWithNoWalletToMakeItIn(t *testing.T) {
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/nonexistent/bus")

	made, err := realMakeCompartment(config.Settings{})

	if err == nil {
		t.Fatalf("realMakeCompartment = %q with no bus to reach, want an error", made)
	}
	if made != "" {
		t.Errorf("realMakeCompartment named %q while failing; nothing was made", made)
	}
}

// TestPlatformWalletViewNamesWhateverItIsGiven covers the fallback beside the
// Secret Service: a name the configuration layer would never produce here still
// gets named back, with nothing claimed about it, rather than being reported as
// some other wallet. Saying "this is what you asked for" remains an answer to
// "which wallet would be used"; inventing a requirement for it would not be.
func TestPlatformWalletViewNamesWhateverItIsGiven(t *testing.T) {
	view := probeWith("linux", nil, nil, "", nil).platformWalletView(config.Settings{}, "something-else")

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
