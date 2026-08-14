//go:build darwin

package main

import (
	"context"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys"
	"github.com/OrbintSoft/sshakku/internal/keys/prompt"
	"github.com/OrbintSoft/sshakku/internal/run"
)

// darwinDialogs is what this platform may have to ask in. The interpreter that
// draws it is part of the base system, so unlike the Linux side there is rarely
// anything to install — but it is still checked for, since a prompter that is
// not there has nowhere to draw either.
//
// Nothing here decides anything on its own — see chooseDialog.
func darwinDialogs(settings config.Settings) []dialog {
	return []dialog{{config.GUIPrompterOsascript, prompt.OsascriptPrompter{
		Runner:  run.ExecRunner{Timeout: settings.InteractiveTimeout},
		Timeout: settings.InteractiveTimeout,
	}}}
}

// newGraphicalPrompter returns the dialog this platform can raise a passphrase
// prompt with, or nil when none can be shown here.
//
// On macOS that means a session with a screen to put one on — which is not the
// same as being on a Mac. Someone logged in over SSH, or booted into single
// user mode, has no window server, and a dialog raised there would be a login
// shell waiting on something that can never arrive; launchctl is what tells
// those apart.
//
// Which dialog is not much of a choice here — the system has one — but "none"
// is still the user's to write, and it means the terminal wherever it appears.
func newGraphicalPrompter(ctx context.Context, settings config.Settings, log keys.Logger) prompt.Prompter {
	if settings.GUIPrompter == config.GUIPrompterNone {
		return nil
	}
	if !prompt.GraphicalSession(ctx, run.ExecRunner{Timeout: settings.CommandTimeout}) {
		return nil
	}
	return chooseDialog(ctx, darwinDialogs(settings), settings.GUIPrompter, prompt.TTYPrompter{}, log)
}
