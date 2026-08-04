//go:build linux

package main

import (
	"fmt"
	"os"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys"
)

// dialog pairs a prompter with the name gui_prompter chooses it by.
type dialog struct {
	name     string
	prompter keys.Prompter
}

// linuxDialogs is the order a graphical session is offered a dialog in. It is a
// table of what a desktop may have, not a judgement about which desktop is in
// use: pinentry comes with GnuPG and draws with whichever toolkit the
// distribution chose for it, so it fits a session SSHakku would otherwise have
// no way to recognise; kdialog and zenity each belong to one desktop, so they
// are asked for after it.
//
// Nothing here decides anything on its own — see newGraphicalPrompter.
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
//
// Which one is then a choice the user may have made. Unmade ("auto"), the first
// one installed is used; made, that one is used or none is — a dialog the user
// did not name is not a substitute for the one they did, and the terminal can
// still ask.
//
// Whichever is found is paired with the terminal, since being installed is not
// the same as being able to run, and a dialog that fails when it is finally
// asked must not take the question down with it.
func newGraphicalPrompter(settings config.Settings, log keys.Logger) keys.Prompter {
	if settings.GUIPrompter == config.GUIPrompterNone {
		return nil
	}
	guiEnv := keys.GUIEnv{
		WaylandDisplay: os.Getenv("WAYLAND_DISPLAY"),
		Display:        os.Getenv("DISPLAY"),
	}
	if !keys.HasGraphicalSession(guiEnv, keys.ExecRunner{}) {
		return nil
	}
	named := settings.GUIPrompter != "" && settings.GUIPrompter != config.GUIPrompterAuto
	for _, d := range linuxDialogs(settings) {
		if named && d.name != settings.GUIPrompter {
			continue
		}
		if d.prompter.Available() {
			return keys.FallbackPrompter{Primary: d.prompter, Fallback: keys.TTYPrompter{}, Log: log}
		}
		if named {
			// Saying which one could not ask, and why, is the difference between
			// something the user can act on and a prompt that simply never came.
			logGUI(log, "gui_prompter names %s, which %s; asking on the terminal", d.name, keys.PrompterUnavailable(d.prompter))
			return nil
		}
	}
	return nil
}

// logGUI records why there is no dialog, when there is something to say.
func logGUI(log keys.Logger, format string, args ...any) {
	if log == nil {
		return
	}
	_ = log.Log("ERROR", fmt.Sprintf(format, args...))
}
