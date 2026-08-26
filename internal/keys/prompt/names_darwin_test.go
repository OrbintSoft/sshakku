//go:build darwin

package prompt

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/run"
	"github.com/OrbintSoft/sshakku/internal/run/runtest"
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
	wontRun := runtest.NewRunner().On(osascriptBin, func(run.Cmd) (run.Result, error) {
		return run.Result{}, notOnPathError{name: osascriptBin}
	})
	prompter := OsascriptPrompter{Runner: wontRun}

	assert.Equal(t, osascriptBin, Name(prompter),
		"the name a message uses has to be the one gui_prompter accepts, or it sends the user "+
			"looking for a program under a name nothing takes")

	log := &fakeLogger{}
	terminal := &namedFake{answer: "typed on the terminal"}
	pass, err := FallbackPrompter{Primary: prompter, Fallback: terminal, Log: log}.Prompt(t.Context(), "id_rsa")
	require.NoError(t, err, "a dialog that could not run must not lose the question")
	assert.Equal(t, "typed on the terminal", pass, "the user is asked on the terminal instead")
	assert.Containsf(t, strings.Join(log.lines, "\n"), osascriptBin,
		"and the log must name the dialog that could not ask: %v", log.lines)
}
