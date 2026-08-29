//go:build windows

package dialog

import (
	"context"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys"
	"github.com/OrbintSoft/sshakku/internal/keys/prompt"
)

// windowsDialogs is what this platform may have to ask in. There is one box and
// SSHakku draws it itself, so unlike the other platforms there is nothing to
// look for: no interpreter to find, nothing to install, and no execution policy
// with anything to say about it.
//
// Nothing here decides anything on its own — see chooseDialog.
func windowsDialogs(settings config.Settings) []dialog {
	return []dialog{{config.GUIPrompterNative, prompt.NativePrompter{
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
// Two things have to hold before there is a dialog at all. The configuration
// must allow one: "none" means the terminal wherever it is written. And the
// session must have a screen — a service, or a session opened to run a
// scheduled job, has a window station with no desktop behind it, and a box
// raised there is one nobody can answer. The third condition the other
// platforms have does not apply here: there is nothing to install to draw this
// box, so it is never the missing piece.
func Graphical(ctx context.Context, settings config.Settings, log keys.Logger) prompt.Prompter {
	if settings.GUIPrompter == config.GUIPrompterNone {
		return nil
	}
	if !thisSessionHasAScreen() {
		return nil
	}
	return chooseDialog(ctx, windowsDialogs(settings), settings.GUIPrompter, prompt.TTYPrompter{}, log)
}
