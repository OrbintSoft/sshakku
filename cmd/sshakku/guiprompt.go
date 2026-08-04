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
// Which one is a choice the user may have made. Made, that one is used or none
// is — a dialog the user did not name is not a substitute for the one they did,
// and the terminal can still ask.
//
// Unmade, every dialog that is installed is asked in turn, ending at the
// terminal. Being installed is not the same as being able to draw: a dialog can
// announce a toolkit and then find no window server behind it, which no list of
// installed programs can tell in advance. One that cannot draw must not take
// the question past the ones that can, and the terminal a login shell started
// from is the last resort rather than the first.
func chooseDialog(dialogs []dialog, want string, terminal keys.Prompter, log keys.Logger) keys.Prompter {
	if want != "" && want != config.GUIPrompterAuto {
		return namedDialog(dialogs, want, terminal, log)
	}
	// Built from the terminal backwards, so each dialog falls back to the next
	// one after it and the last of them falls back to the terminal.
	asked, found := terminal, false
	for i := len(dialogs) - 1; i >= 0; i-- {
		if !dialogs[i].prompter.Available() {
			continue
		}
		asked, found = keys.FallbackPrompter{Primary: dialogs[i].prompter, Fallback: asked, Log: log}, true
	}
	if !found {
		return nil
	}
	return asked
}

// namedDialog returns the one dialog the configuration asked for, paired with
// the terminal, or nil when it cannot ask here.
func namedDialog(dialogs []dialog, want string, terminal keys.Prompter, log keys.Logger) keys.Prompter {
	for _, d := range dialogs {
		if d.name != want {
			continue
		}
		if d.prompter.Available() {
			return keys.FallbackPrompter{Primary: d.prompter, Fallback: terminal, Log: log}
		}
		// Saying which one could not ask, and why, is the difference between
		// something the user can act on and a prompt that simply never came.
		logGUI(log, "gui_prompter names %s, which %s; asking on the terminal", d.name, keys.PrompterUnavailable(d.prompter))
		return nil
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
