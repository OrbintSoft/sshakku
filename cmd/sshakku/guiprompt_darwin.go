//go:build darwin

package main

import (
	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys"
)

// newGraphicalPrompter returns the dialog this platform can raise a passphrase
// prompt with, or nil when none can be shown here.
//
// On macOS that means a session with a screen to put one on — which is not the
// same as being on a Mac. Someone logged in over SSH, or booted into single
// user mode, has no window server, and a dialog raised there would be a login
// shell waiting on something that can never arrive; launchctl is what tells
// those apart. The interpreter that draws it is part of the base system, so
// unlike the Linux side there is rarely anything to install — but it is still
// checked for, since a prompter that is not there has nowhere to draw either.
//
// Which dialog is not much of a choice here — the system has one — but "none"
// is still the user's to write, and it means the terminal wherever it appears.
//
// The dialog that is found is still paired with the terminal: being installed is
// not the same as being able to run, and a dialog that fails when it is finally
// asked must not take the question down with it.
func newGraphicalPrompter(settings config.Settings, log keys.Logger) keys.Prompter {
	switch settings.GUIPrompter {
	case config.GUIPrompterNone:
		return nil
	case "", config.GUIPrompterAuto, config.GUIPrompterOsascript:
	default:
		// A name this system has no dialog for. The configuration layer refuses
		// those already, so this only catches a Settings built by hand.
		return nil
	}
	dialog := keys.OsascriptPrompter{
		Runner:  keys.ExecRunner{Timeout: settings.InteractiveTimeout},
		Timeout: settings.InteractiveTimeout,
	}
	if !keys.GraphicalSession(keys.ExecRunner{Timeout: settings.CommandTimeout}) || !dialog.Available() {
		return nil
	}
	return keys.FallbackPrompter{Primary: dialog, Fallback: keys.TTYPrompter{}, Log: log}
}
