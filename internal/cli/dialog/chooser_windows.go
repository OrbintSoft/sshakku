//go:build windows

package dialog

import (
	"context"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys"
	"github.com/OrbintSoft/sshakku/internal/keys/prompt"
	"github.com/OrbintSoft/sshakku/internal/run"
)

// windowsDialogs is what this platform may have to ask in. There is one box and
// it is drawn by a PowerShell host, which is part of the system rather than
// something to install — but it is still looked for, since a host that is not
// there has nothing to draw with either.
//
// Nothing here decides anything on its own — see chooseDialog.
func windowsDialogs(settings config.Settings) []dialog {
	return []dialog{{config.GUIPrompterPowerShell, prompt.PowerShellPrompter{
		Runner:  run.ExecRunner{Timeout: settings.InteractiveTimeout},
		Timeout: settings.InteractiveTimeout,
	}}}
}

// thisSessionHasAScreen is what the system says about the session this process
// is in. It is a variable so a test can put the wiring in a session this
// machine cannot be put into: a desk always has a screen and a build runner
// never does, and each would otherwise leave the other's answer unexercised.
var thisSessionHasAScreen = prompt.GraphicalSession

// Graphical returns the dialog this platform can raise a passphrase prompt
// with, or nil when none can be shown here — in which case the caller asks on
// the terminal.
//
// Three things have to hold before there is a dialog at all, as on the other
// platforms. The configuration must allow one: "none" means the terminal
// wherever it is written. The session must have a screen — a service, or a
// session opened to run a scheduled job, has a window station with no desktop
// behind it, and a box raised there is one nobody can answer. And a host must
// be installed to draw it.
func Graphical(ctx context.Context, settings config.Settings, log keys.Logger) prompt.Prompter {
	if settings.GUIPrompter == config.GUIPrompterNone {
		return nil
	}
	if !thisSessionHasAScreen() {
		return nil
	}
	return chooseDialog(ctx, windowsDialogs(settings), settings.GUIPrompter, prompt.TTYPrompter{}, log)
}
