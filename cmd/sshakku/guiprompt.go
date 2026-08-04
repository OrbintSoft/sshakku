package main

import (
	"fmt"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys"
)

// dialog pairs a prompter with the name gui_prompter chooses it by.
type dialog struct {
	name     string
	prompter keys.Prompter
}

// chooseDialog returns the prompter a session with a screen is asked in, given
// the dialogs this platform may have (most fitting first) and the terminal to
// fall back on. Nil means there is no dialog to raise here and the caller asks
// on the terminal itself.
//
// Which one is a choice the user may have made. Unmade ("auto" or nothing), the
// first one installed is used; made, that one is used or none is — a dialog the
// user did not name is not a substitute for the one they did, and the terminal
// can still ask.
//
// Whichever is found is paired with the terminal, since being installed is not
// the same as being able to run, and a dialog that fails when it is finally
// asked must not take the question down with it.
func chooseDialog(dialogs []dialog, want string, terminal keys.Prompter, log keys.Logger) keys.Prompter {
	named := want != "" && want != config.GUIPrompterAuto
	for _, d := range dialogs {
		if named && d.name != want {
			continue
		}
		if d.prompter.Available() {
			return keys.FallbackPrompter{Primary: d.prompter, Fallback: terminal, Log: log}
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
