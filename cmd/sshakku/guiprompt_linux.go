//go:build linux

package main

import (
	"os"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys"
)

// newGraphicalPrompter returns the dialog this platform can raise a passphrase
// prompt with, or nil when none can be shown here.
//
// On Linux that means a reachable display server — a Wayland compositor, or an X
// server that answers — and kdialog installed to draw on it. Both have to hold:
// a session with no prompter installed cannot be asked in, and an installed
// prompter with no session has nowhere to draw.
func newGraphicalPrompter(settings config.Settings) keys.Prompter {
	runner := keys.ExecRunner{Timeout: settings.CommandTimeout}
	kdialog := keys.KDialogPrompter{Runner: runner, Timeout: settings.InteractiveTimeout}
	guiEnv := keys.GUIEnv{
		WaylandDisplay: os.Getenv("WAYLAND_DISPLAY"),
		Display:        os.Getenv("DISPLAY"),
	}
	if !keys.GUIAvailable(guiEnv, keys.ExecRunner{}, kdialog) {
		return nil
	}
	return kdialog
}
