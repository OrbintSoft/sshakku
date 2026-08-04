//go:build darwin

package keys

import (
	"errors"
	"strings"
	"testing"
)

// TestTheDialogThatFailsIsNamedByItsOwnName verifies the part of F37 that only
// shows up once something goes wrong: a dialog that cannot ask hands the
// question on and the log says which one failed. The name it is called there
// has to be the one the user would type into `gui_prompter`, or the message
// sends them looking for a program under a name nothing accepts.
//
// This is the macOS half of the check the Linux job already makes for kdialog
// and zenity; the dialog differs, the promise does not.
func TestTheDialogThatFailsIsNamedByItsOwnName(t *testing.T) {
	// A dialog whose interpreter is not there at all: the runner answers the
	// way it does when exec fails, which is what the fallback exists for.
	wontRun := newFakeRunner().on(osascriptBin, func(Cmd) (Result, error) {
		return Result{}, errors.New("exec: \"" + osascriptBin + "\": executable file not found in $PATH")
	})
	prompter := OsascriptPrompter{Runner: wontRun}

	if got := PrompterName(prompter); got != osascriptBin {
		t.Errorf("PrompterName = %q, want %q — the name gui_prompter accepts", got, osascriptBin)
	}

	log := &fakeLogger{}
	terminal := &namedFake{answer: "typed on the terminal"}
	pass, err := FallbackPrompter{Primary: prompter, Fallback: terminal, Log: log}.Prompt("id_rsa")
	if err != nil || pass != "typed on the terminal" {
		t.Fatalf("Prompt = (%q, %v), want the terminal's answer", pass, err)
	}
	named := false
	for _, line := range log.lines {
		if strings.Contains(line, osascriptBin) {
			named = true
		}
	}
	if !named {
		t.Errorf("log = %v, want the dialog that could not ask named in it", log.lines)
	}
}
