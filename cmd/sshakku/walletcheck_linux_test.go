//go:build linux

package main

import (
	"testing"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/diagnose"
	"github.com/OrbintSoft/sshakku/internal/paths"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withLook fixes what looking at the session bus found, and whether the session
// has a screen, so every combination of the two can be described from a machine
// that has neither. Looking at the session bus is a thing only Linux does, so
// only Linux's tests need it.
func (p walletProbe) withLook(look secretServiceLook, hasScreen bool) walletProbe {
	p.look = func(string, string) secretServiceLook { return look }
	p.hasScreen = hasScreen
	return p
}

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

	assert.Equal(t, config.SecretBackendSecretService, view.Backend,
		"an unconfigured Linux machine uses the freedesktop Secret Service")
	req := requirement(t, view, "session bus")
	assert.True(t, req.Present, "the bus the shell exported is there")
	assert.Equal(t, "unix:path=/run/bus", req.Detail, "and the report names the address it found")
}

// TestDoctorReportsAMissingSessionBus covers the other answer: the wallet is
// there to be named, but the bus it is reached over is not, which is a piece
// the user can go and provide — unlike a wallet the system has not got.
func TestDoctorReportsAMissingSessionBus(t *testing.T) {
	settings := config.Settings{SecretBackend: config.SecretBackendSecretService}

	view := walletView(settings, probeWith("linux", nil, nil, "", nil))

	req := requirement(t, view, "session bus")
	assert.False(t, req.Present, "a bus whose address is unset is not one that is there")
	assert.Contains(t, req.Detail, "DBUS_SESSION_BUS_ADDRESS is unset",
		"and the report must name what the user has to set")
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

	assert.Equal(t, config.KeePassXCRouteSecretService, view.Route, "the route Linux picks on its own")
	req := requirement(t, view, "session bus")
	assert.True(t, req.Present, "reaching it that way needs the bus, and the bus is there")
	assert.Equal(t, "unix:path=/run/bus", req.Detail, "and the report names the address it found")
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
	assert.False(t, req.Present, "a compartment that cannot be created is not one that is there")
	assert.False(t, req.Undetermined, "and the wallet was reachable, so this is known rather than unasked")

	findings := diagnose.WalletFindings(view)
	require.Len(t, findings, 1, "a wallet that can hold nothing is one thing wrong, and it must reach the findings")
	assert.Contains(t, findings[0], "compartment", "the finding must name the piece that is missing")
}

// TestDoctorSaysACompartmentItCanMakeIsNotThereYet verifies F42 where the
// session can make one: the report says the compartment is not there, and does
// not call that a fault. Both halves matter and they are separate claims — one
// that would appear by itself at the first passphrase saved is nothing wrong
// with the machine, but it is still not something the report may call found.
func TestDoctorSaysACompartmentItCanMakeIsNotThereYet(t *testing.T) {
	settings := config.Settings{SecretBackend: config.SecretBackendSecretService}
	probe := probeWith("linux", nil, nil, "unix:path=/run/bus", nil).
		withLook(secretServiceLook{running: true}, true)

	view := walletView(settings, probe)

	req := requirement(t, view, "compartment")
	assert.False(t, req.Present, "one that would appear at the first passphrase saved is still not there yet")
	assert.True(t, req.Fixable, "but this session can go and provide it")
	assert.Empty(t, diagnose.WalletFindings(view),
		"and something the machine can put right by itself is not a fault to report")
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

	assert.Contains(t, req.Detail, "my-own",
		"the compartment described must be the one entries would go into, which the user named")
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

	assert.False(t, requirement(t, view, "secret service").Present,
		"a bus with nothing answering on it has no wallet on it")
	for _, req := range view.Requirements {
		assert.NotEqual(t, "compartment", req.Name,
			"this route's entries live in a database the user opened, so the desktop has no compartment to describe")
	}
}

// TestALookThatCouldNotBeTakenSaysSo verifies F41 at the seam where the report
// meets the bus: a look that fails is not an answer. Reporting it as one would
// have the report state, as fact, that a wallet is not there — on the strength
// of never having managed to ask.
func TestALookThatCouldNotBeTakenSaysSo(t *testing.T) {
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/nonexistent/bus")

	look := realSecretServiceLook("sshakku", "sshakku")

	assert.True(t, look.lookFailed, "a look that never reached a bus must say it failed")

	req := serviceRequirement(look)
	assert.True(t, req.Undetermined, "never having managed to ask is not an answer")
	assert.False(t, req.Present, "and it is certainly not a yes")
}

// TestMakingACompartmentWithNoWalletToMakeItIn verifies F42 where it is easiest
// to get wrong: a repair that could not be performed has to say so. A maker that
// swallowed the failure would have --fix report a compartment it never made, and
// the next login would be the one to find out.
func TestMakingACompartmentWithNoWalletToMakeItIn(t *testing.T) {
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/nonexistent/bus")

	made, err := realMakeCompartment(config.Settings{})

	assert.Error(t, err, "a repair that could not be performed must say so, or the next login finds out")
	assert.Empty(t, made, "and nothing may be named as made when nothing was")
}

// TestPlatformWalletViewNamesWhateverItIsGiven covers the fallback beside the
// Secret Service: a name the configuration layer would never produce here still
// gets named back, with nothing claimed about it, rather than being reported as
// some other wallet. Saying "this is what you asked for" remains an answer to
// "which wallet would be used"; inventing a requirement for it would not be.
func TestPlatformWalletViewNamesWhateverItIsGiven(t *testing.T) {
	view := probeWith("linux", nil, nil, "", nil).platformWalletView(config.Settings{}, "something-else")

	assert.Equal(t, "something-else", view.Backend,
		"naming back what was asked for is still an answer to which wallet would be used")
	assert.Empty(t, view.Requirements, "but inventing requirements for a wallet nothing is known about would not be")
}

// discardLogger is the session log a test has no use for.
type discardLogger struct{}

func (discardLogger) Log(string, string) error { return nil }
