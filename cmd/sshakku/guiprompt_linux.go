//go:build linux

package main

import (
	"os"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys"
)

// linuxPrompters is the order a graphical session is offered a dialog in. It is
// a table of what a desktop may have, not a judgement about which desktop is in
// use: pinentry comes with GnuPG and draws with whichever toolkit the
// distribution chose for it, so it fits a session SSHakku would otherwise have
// no way to recognise; kdialog and zenity are asked for after it because they
// belong to one desktop each.
//
// Nothing here decides anything on its own — the first one installed is the one
// used, and a session with none of them is asked on the terminal.
func linuxPrompters(settings config.Settings) []keys.Prompter {
	runner := keys.ExecRunner{Timeout: settings.CommandTimeout}
	return []keys.Prompter{
		keys.PinentryPrompter{Timeout: settings.InteractiveTimeout},
		keys.KDialogPrompter{Runner: runner, Timeout: settings.InteractiveTimeout},
	}
}

// newGraphicalPrompter returns the dialog this platform can raise a passphrase
// prompt with, or nil when none can be shown here.
//
// On Linux that means a reachable display server — a Wayland compositor, or an X
// server that answers — and one of the dialogs installed to draw on it. Both
// have to hold: a session with no dialog installed cannot be asked in, and an
// installed dialog with no session has nowhere to draw.
//
// The dialog that is found is still paired with the terminal: being installed is
// not the same as being able to run, and a dialog that fails when it is finally
// asked must not take the question down with it.
func newGraphicalPrompter(settings config.Settings, log keys.Logger) keys.Prompter {
	guiEnv := keys.GUIEnv{
		WaylandDisplay: os.Getenv("WAYLAND_DISPLAY"),
		Display:        os.Getenv("DISPLAY"),
	}
	if !keys.HasGraphicalSession(guiEnv, keys.ExecRunner{}) {
		return nil
	}
	for _, p := range linuxPrompters(settings) {
		if p.Available() {
			return keys.FallbackPrompter{Primary: p, Fallback: keys.TTYPrompter{}, Log: log}
		}
	}
	return nil
}
