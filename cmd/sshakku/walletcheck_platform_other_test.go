//go:build !linux

package main

import (
	"runtime"
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

// discardLogger is the session log a test has no use for.
type discardLogger struct{}

func (discardLogger) Log(string, string) error { return nil }
