package keys

import (
	"errors"
	"strings"
	"testing"
)

// TestADialogThatFailsIsNamedByItsOwnName verifies the part of F37 that only
// shows up once something goes wrong: a dialog that cannot ask hands the
// question to the terminal and the log says which one failed. The name it is
// called there has to be the one the user would type into `gui_prompter`, or
// the message sends them looking for a program under a name nothing accepts.
func TestADialogThatFailsIsNamedByItsOwnName(t *testing.T) {
	// A dialog whose program is not there at all: the runner answers the way it
	// does when exec fails, which is what the fallback exists for.
	wontRun := func(name string) *fakeRunner {
		return newFakeRunner().on(name, func(Cmd) (Result, error) {
			return Result{}, errors.New("exec: \"" + name + "\": executable file not found in $PATH")
		})
	}
	cases := []struct {
		name     string
		prompter Prompter
	}{
		{"kdialog", KDialogPrompter{Runner: wontRun("kdialog")}},
		{"zenity", ZenityPrompter{Runner: wontRun("zenity")}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := PrompterName(c.prompter); got != c.name {
				t.Errorf("PrompterName = %q, want %q — the name gui_prompter accepts", got, c.name)
			}

			log := &fakeLogger{}
			terminal := &namedFake{answer: "typed on the terminal"}
			pass, err := FallbackPrompter{Primary: c.prompter, Fallback: terminal, Log: log}.Prompt("id_rsa")
			if err != nil || pass != "typed on the terminal" {
				t.Fatalf("Prompt = (%q, %v), want the terminal's answer", pass, err)
			}
			named := false
			for _, line := range log.lines {
				if strings.Contains(line, c.name) {
					named = true
				}
			}
			if !named {
				t.Errorf("log = %v, want the dialog that could not ask named in it", log.lines)
			}
		})
	}
}
