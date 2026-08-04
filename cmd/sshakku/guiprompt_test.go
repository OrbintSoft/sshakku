package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys"
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

	p := chooseDialog([]dialog{
		{config.GUIPrompterPinentry, cannotDraw},
		{config.GUIPrompterKDialog, notInstalled},
		{config.GUIPrompterZenity, canDraw},
	}, "", terminal, log)
	if p == nil {
		t.Fatal("chooseDialog = nil with two dialogs installed, want the one the user can be asked in")
	}

	pass, err := p.Prompt("id_a")
	if err != nil {
		t.Fatalf("Prompt = %v, want the passphrase from the dialog that can draw", err)
	}
	if pass != typed {
		t.Errorf("Prompt = %q, want %q: the dialog that could not draw took the question past the one that could", pass, typed)
	}
	if terminal.asked != 0 {
		t.Errorf("the terminal was asked %d time(s), want none: the user is at a screen with a working dialog on it", terminal.asked)
	}
	if said := strings.Join(log.lines, "\n"); !strings.Contains(said, "pinentry") {
		t.Errorf("the log says %q, want the dialog that could not ask named in it", said)
	}
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

	p := chooseDialog([]dialog{
		{config.GUIPrompterPinentry, cannotDraw},
		{config.GUIPrompterZenity, canDraw},
	}, config.GUIPrompterPinentry, terminal, nil)
	if p == nil {
		t.Fatal("chooseDialog = nil with the named dialog installed, want it asked in")
	}

	pass, err := p.Prompt("id_a")
	if err != nil {
		t.Fatalf("Prompt = %v, want the terminal's answer", err)
	}
	if pass != onTheTerminal {
		t.Errorf("Prompt = %q, want %q: a dialog the user did not name is not a substitute for the one they did", pass, onTheTerminal)
	}
}

// TestClosingTheFirstDialogDoesNotRaiseTheNext verifies F38 where F37 now
// reaches: closing a dialog without answering is an answer, so a chain of
// dialogs must not turn one closed window into the next one. A user who shuts
// the prompt would otherwise be answering the same question once per program
// their desktop happens to have installed.
func TestClosingTheFirstDialogDoesNotRaiseTheNext(t *testing.T) {
	closed := &fakeDialog{name: "pinentry", installed: true, err: keys.ErrPromptCanceled}
	next := &fakeDialog{name: "zenity", installed: true, answer: "the-one-nobody-asked-for"}
	terminal := &fakeDialog{name: "the terminal", installed: true, answer: "the-one-typed-on-the-terminal"}

	p := chooseDialog([]dialog{
		{config.GUIPrompterPinentry, closed},
		{config.GUIPrompterZenity, next},
	}, "", terminal, nil)

	if _, err := p.Prompt("id_a"); !errors.Is(err, keys.ErrPromptCanceled) {
		t.Errorf("Prompt = %v, want the dismissal passed on as the answer it is", err)
	}
	if next.asked != 0 || terminal.asked != 0 {
		t.Errorf("after the dialog was closed, zenity was asked %d time(s) and the terminal %d, want none of either", next.asked, terminal.asked)
	}
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

func (d *fakeDialog) Prompt(string) (string, error) {
	d.asked++
	if d.err != nil {
		return "", d.err
	}
	return d.answer, nil
}

func (d *fakeDialog) Available() bool { return d.installed }

func (d *fakeDialog) Name() string { return d.name }

var _ keys.Prompter = (*fakeDialog)(nil)
