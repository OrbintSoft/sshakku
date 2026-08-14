package dialog

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys/prompt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestADialogThatCannotDrawIsFollowedByOneThatCan verifies F37 for the case a
// list of installed programs cannot see coming: a dialog is installed, says it
// can ask, and then puts no window anywhere — GnuPG's pinentry does exactly
// this when the toolkit it announces has no prompter behind it. The desktop
// still provides a dialog that works, so being sent to a terminal instead is
// the promise not being kept: the user is sitting in front of a screen, and the
// terminal a login shell was started from may be behind the window they are
// looking at, or gone.
//
// The dialogs are supplied as what a table of them is — programs that may or
// may not be installed, and that answer or fail once they are finally asked.
// Which one the user ends up answering is what is being judged, and that is
// left entirely to the code under test.
func TestADialogThatCannotDrawIsFollowedByOneThatCan(t *testing.T) {
	const typed = "the-one-typed-into-the-second-dialog"

	cannotDraw := &fakeDialog{name: "pinentry", installed: true, err: errors.New("Inappropriate ioctl for device")}
	notInstalled := &fakeDialog{name: "kdialog"}
	canDraw := &fakeDialog{name: "zenity", installed: true, answer: typed}
	terminal := &fakeDialog{name: "the terminal", installed: true, answer: "the-one-typed-on-the-terminal"}
	log := &recordingLogger{}

	p := chooseDialog(t.Context(), []dialog{
		{"pinentry", cannotDraw},
		{"kdialog", notInstalled},
		{"zenity", canDraw},
	}, "", terminal, log)
	require.NotNil(t, p, "with two dialogs installed the user must be asked in one of them")

	pass, err := p.Prompt(t.Context(), "id_a")
	require.NoError(t, err, "the dialog that can draw must answer")
	assert.Equal(t, typed, pass,
		"a dialog that could not draw must not take the question past one that could")
	assert.Zero(t, terminal.asked,
		"the user is sitting at a screen with a working dialog on it, so the terminal is not for asking")
	assert.Contains(t, strings.Join(log.lines, "\n"), "pinentry",
		"the dialog that could not ask must be named where the user can find it")
}

// TestANamedDialogThatCannotDrawGoesToTheTerminal covers the other half of the
// same sentence in F37, which must not move when the one above is kept: a user
// who named their dialog gets that one or the terminal. Passing the question to
// a dialog they did not choose would be overruling the choice they made.
func TestANamedDialogThatCannotDrawGoesToTheTerminal(t *testing.T) {
	const onTheTerminal = "the-one-typed-on-the-terminal"

	cannotDraw := &fakeDialog{name: "pinentry", installed: true, err: errors.New("Inappropriate ioctl for device")}
	canDraw := &fakeDialog{name: "zenity", installed: true, answer: "the-one-nobody-asked-for"}
	terminal := &fakeDialog{name: "the terminal", installed: true, answer: onTheTerminal}

	p := chooseDialog(t.Context(), []dialog{
		{"pinentry", cannotDraw},
		{"zenity", canDraw},
	}, "pinentry", terminal, nil)
	require.NotNil(t, p, "the dialog the user named is installed, so it is the one asked in")

	pass, err := p.Prompt(t.Context(), "id_a")
	require.NoError(t, err, "the terminal must answer when the named dialog cannot")
	assert.Equal(t, onTheTerminal, pass,
		"a dialog the user did not name is not a substitute for the one they did")
}

// TestADialogThisPlatformHasNotGotAsksOnTheTerminal covers a name that is valid
// somewhere and has no dialog behind it here — a configuration written on one
// machine and carried to another, or to another operating system. The terminal
// asks, and no dialog this platform does have is quietly substituted for the
// one that was written down.
func TestADialogThisPlatformHasNotGotAsksOnTheTerminal(t *testing.T) {
	here := &fakeDialog{name: "zenity", installed: true, answer: "the-one-nobody-asked-for"}

	assert.Nil(t, chooseDialog(t.Context(), []dialog{{"zenity", here}}, "kdialog", &fakeDialog{}, nil),
		"a name this platform has no dialog for sends the question to the terminal, not to another dialog")
}

// TestANamedDialogThatIsNotInstalledIsSaidSo verifies the sentence F37 makes
// about a dialog the user chose and does not have: they are asked on the
// terminal, and told which one could not ask. Being asked somewhere they were
// not expecting, with nothing said about why, leaves them to guess at a setting
// that looks like it is being ignored.
func TestANamedDialogThatIsNotInstalledIsSaidSo(t *testing.T) {
	t.Run("the log names the one that could not ask", func(t *testing.T) {
		log := &recordingLogger{}

		assert.Nil(t, chooseDialog(t.Context(), []dialog{{"pinentry", &fakeDialog{name: "pinentry"}}}, "pinentry", &fakeDialog{}, log),
			"a dialog that is not installed cannot ask, so the terminal does")
		assert.Contains(t, strings.Join(log.lines, "\n"), "pinentry",
			"and the user must be told which one could not, or the setting looks ignored")
	})

	t.Run("with nowhere to write it, the answer is the same", func(t *testing.T) {
		// A caller that keeps no log still gets a prompt: what is written down
		// is a courtesy, and losing it must not cost the user the question.
		assert.Nil(t, chooseDialog(t.Context(), []dialog{{"pinentry", &fakeDialog{name: "pinentry"}}}, "pinentry", &fakeDialog{}, nil),
			"what is written down is a courtesy; losing it must not cost the user the question")
	})
}

// TestWritingAutoIsTheSameAsNamingNoDialogAtAll covers the value a user writes
// to say "choose for me" out loud. It is a setting, not a dialog's name, and
// read as one it matches nothing installed — so writing down the default would
// take away every dialog on the desktop and send the prompt to the terminal,
// which is the one outcome the setting exists to avoid.
func TestWritingAutoIsTheSameAsNamingNoDialogAtAll(t *testing.T) {
	const inTheDialog = "the-one-typed-into-the-dialog"

	installed := &fakeDialog{name: "zenity", installed: true, answer: inTheDialog}
	terminal := &fakeDialog{name: "the terminal", installed: true, answer: "the-one-typed-on-the-terminal"}

	p := chooseDialog(t.Context(), []dialog{{"zenity", installed}}, config.GUIPrompterAuto, terminal, nil)
	require.NotNil(t, p, "auto means choose one for me, not ask nowhere")

	pass, err := p.Prompt(t.Context(), "id_a")
	require.NoError(t, err, "the dialog must answer")
	assert.Equal(t, inTheDialog, pass, "and the dialog that is installed is the one that asks")
	assert.Zero(t, terminal.asked, "the terminal is not where a user with a screen and a dialog is asked")
}

// TestClosingTheFirstDialogDoesNotRaiseTheNext verifies F38 where F37 now
// reaches: closing a dialog without answering is an answer, so a chain of
// dialogs must not turn one closed window into the next one. A user who shuts
// the prompt would otherwise be answering the same question once per program
// their desktop happens to have installed.
func TestClosingTheFirstDialogDoesNotRaiseTheNext(t *testing.T) {
	closed := &fakeDialog{name: "pinentry", installed: true, err: prompt.ErrCanceled}
	next := &fakeDialog{name: "zenity", installed: true, answer: "the-one-nobody-asked-for"}
	terminal := &fakeDialog{name: "the terminal", installed: true, answer: "the-one-typed-on-the-terminal"}

	p := chooseDialog(t.Context(), []dialog{
		{"pinentry", closed},
		{"zenity", next},
	}, "", terminal, nil)

	_, err := p.Prompt(t.Context(), "id_a")
	assert.ErrorIs(t, err, prompt.ErrCanceled, "closing a dialog is an answer, and must be passed on as one")
	assert.Zero(t, next.asked, "the next dialog must not be raised over a window the user just shut")
	assert.Zero(t, terminal.asked, "nor the terminal")
}

// recordingLogger keeps what was written, so a test can read the line a user
// would find in the session log.
type recordingLogger struct{ lines []string }

func (r *recordingLogger) Log(level, message string) error {
	r.lines = append(r.lines, level+" "+message)
	return nil
}

// fakeDialog is one entry of a dialog table: a program that is installed or
// not, and that answers or fails once it is finally asked — the two halves that
// being on PATH cannot tell apart.
type fakeDialog struct {
	name      string
	installed bool
	answer    string
	err       error
	asked     int
}

func (d *fakeDialog) Prompt(context.Context, string) (string, error) {
	d.asked++
	if d.err != nil {
		return "", d.err
	}
	return d.answer, nil
}

func (d *fakeDialog) Available(context.Context) bool { return d.installed }

func (d *fakeDialog) Name() string { return d.name }

var _ prompt.Prompter = (*fakeDialog)(nil)
