//go:build linux

package main

import (
	"context"
	"os"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys"
)

// linuxDialogs is the order a graphical session is offered a dialog in. It is a
// table of what a desktop may have, not a judgement about which desktop is in
// use: pinentry comes with GnuPG and draws with whichever toolkit the
// distribution chose for it, so it fits a session SSHakku would otherwise have
// no way to recognise; kdialog and zenity each belong to one desktop, so they
// are asked for after it.
//
// Nothing here decides anything on its own — see chooseDialog.
func linuxDialogs(settings config.Settings) []dialog {
	runner := keys.ExecRunner{Timeout: settings.CommandTimeout}
	return []dialog{
		{config.GUIPrompterPinentry, keys.PinentryPrompter{Timeout: settings.InteractiveTimeout, ProbeTimeout: settings.CommandTimeout}},
		{config.GUIPrompterKDialog, keys.KDialogPrompter{Runner: runner, Timeout: settings.InteractiveTimeout}},
		{config.GUIPrompterZenity, keys.ZenityPrompter{Runner: runner, Timeout: settings.InteractiveTimeout}},
	}
}

// newGraphicalPrompter returns the dialog this platform can raise a passphrase
// prompt with, or nil when none can be shown here — in which case the caller
// asks on the terminal.
//
// Three things have to hold before there is a dialog at all. The configuration
// must allow one: "none" means the terminal wherever it is written. There must
// be a display server — a Wayland compositor, or an X server that answers —
// because an installed dialog with no session has nowhere to draw. And a dialog
// must be there to ask in — which is more than being installed, since one of
// pinentry's builds draws on a terminal and would take the question somewhere
// nobody is looking.
func newGraphicalPrompter(ctx context.Context, settings config.Settings, log keys.Logger) keys.Prompter {
	if settings.GUIPrompter == config.GUIPrompterNone {
		return nil
	}
	guiEnv := keys.GUIEnv{
		WaylandDisplay: os.Getenv("WAYLAND_DISPLAY"),
		Display:        os.Getenv("DISPLAY"),
	}
	if !keys.HasGraphicalSession(ctx, guiEnv, keys.ExecRunner{}) {
		return nil
	}
	return chooseDialog(ctx, linuxDialogs(settings), settings.GUIPrompter, keys.TTYPrompter{}, log)
}
