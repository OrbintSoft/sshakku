//go:build windows

package prompt

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"golang.org/x/sys/windows"
)

// The failures these tests hand their seams. Each stands for a real one the
// code under test cannot be made to produce on demand.
var (
	errNotAConsole        = errors.New("not a console")
	errRefused            = errors.New("refused")
	errTheHandleIsInvalid = errors.New("the handle is invalid")
)

// The handles a fake console answers to. They are never opened: what stands in
// for a console is the pair of seams below, and these only tell the two halves
// apart in what the fake records.
const (
	fakeIn  = windows.Handle(1)
	fakeOut = windows.Handle(2)
)

// fakeConsole is a console nobody has to type into. It records what was written
// and every mode it was put into, in order, which is what lets a test say that
// echo went off *before* the question was asked rather than merely that it went
// off.
type fakeConsole struct {
	written  []string
	modes    []uint32
	mode     uint32
	line     string
	readErr  error
	modeErr  error
	setErr   error
	writeErr error
	closed   bool
}

// install points the seams at this fake for the duration of the test.
func (c *fakeConsole) install(t *testing.T) {
	t.Helper()
	oldOpen, oldMode, oldSet := openConsole, consoleModeOf, setConsoleMode
	oldRead, oldWrite := readConsole, writeConsole
	t.Cleanup(func() {
		openConsole, consoleModeOf, setConsoleMode = oldOpen, oldMode, oldSet
		readConsole, writeConsole = oldRead, oldWrite
	})

	openConsole = func() (console, func(), error) {
		return console{in: fakeIn, out: fakeOut}, func() { c.closed = true }, nil
	}
	consoleModeOf = func(_ windows.Handle, mode *uint32) error {
		if c.modeErr != nil {
			return c.modeErr
		}
		*mode = c.mode
		return nil
	}
	setConsoleMode = func(_ windows.Handle, mode uint32) error {
		if c.setErr != nil {
			return c.setErr
		}
		c.mode = mode
		c.modes = append(c.modes, mode)
		// Recorded in the same sequence as what was written, so a test can say
		// echo went off *before* the question was asked and not merely that
		// both happened.
		if mode&windows.ENABLE_ECHO_INPUT == 0 {
			c.written = append(c.written, "<echo off>")
		} else {
			c.written = append(c.written, "<echo on>")
		}
		return nil
	}
	readConsole = func(windows.Handle) (string, error) {
		// The line and the failure are independent: a console can hand over
		// what was typed and still report trouble, and which of the two the
		// caller acts on is the thing worth asserting.
		c.written = append(c.written, "<read>")
		return c.line, c.readErr
	}
	writeConsole = func(_ windows.Handle, s string) error {
		if c.writeErr != nil {
			return c.writeErr
		}
		c.written = append(c.written, s)
		return nil
	}
}

// F7, F29: a passphrase typed at the console is never shown, and echo is off
// before the question appears — so seeing the prompt is itself the guarantee
// that what follows is not being printed. The mode is put back afterwards,
// because the console belongs to the session and not to this process.
func TestAPassphraseIsAskedForWithTheEchoAlreadyOff(t *testing.T) {
	con := &fakeConsole{mode: windows.ENABLE_ECHO_INPUT | windows.ENABLE_LINE_INPUT, line: "hunter2\r\n"}
	con.install(t)

	answer, err := ReadTTYLine("Enter passphrase for id_ed25519: ", true)

	require.NoError(t, err)
	assert.Equal(t, "hunter2", answer, "the terminator the user pressed is not part of the answer")
	require.Len(t, con.modes, 2, "echo off, and put back")
	assert.Zero(t, con.modes[0]&windows.ENABLE_ECHO_INPUT, "echo is off for the typing")
	assert.NotZero(t, con.modes[0]&windows.ENABLE_LINE_INPUT,
		"and only echo is off: line editing is what a person expects while typing")
	assert.Equal(t, uint32(windows.ENABLE_ECHO_INPUT|windows.ENABLE_LINE_INPUT), con.modes[1],
		"the console is handed back as it was found")
	assert.Equal(t,
		[]string{"<echo off>", "Enter passphrase for id_ed25519: ", "<read>", "\r\n", "<echo on>"},
		con.written,
		"echo off, then the question, the answer, the newline the console did not echo, and echo back on")
	assert.True(t, con.closed, "the console is let go of")
}

// F38: closing the input instead of answering is a decision, and it comes back
// as one. An answer of nothing at all is a different thing — the user pressed
// the key — and must not be mistaken for it.
func TestClosingTheInputIsADecisionAndAnEmptyAnswerIsNot(t *testing.T) {
	t.Run("input closed", func(t *testing.T) {
		con := &fakeConsole{line: ""}
		con.install(t)
		_, err := ReadTTYLine("passphrase: ", true)
		assert.ErrorIs(t, err, ErrCanceled, "nothing at all came back: the input was closed")
	})

	t.Run("answered with nothing", func(t *testing.T) {
		con := &fakeConsole{line: "\r\n"}
		con.install(t)
		answer, err := ReadTTYLine("passphrase: ", true)
		require.NoError(t, err, "pressing the key is an answer, however empty")
		assert.Empty(t, answer)
	})
}

// F29, F40: where there is no console there is nobody to ask, and that is
// reported at once rather than waited on. It is ErrNoTerminal, which callers
// read as "could not ask this round" instead of as something to go and fix.
func TestNoConsoleIsReportedAsNobodyToAsk(t *testing.T) {
	oldOpen := openConsole
	t.Cleanup(func() { openConsole = oldOpen })
	openConsole = func() (console, func(), error) {
		return console{}, func() {}, errTheHandleIsInvalid
	}

	_, err := ReadTTYLine("passphrase: ", true)

	require.ErrorIs(t, err, ErrNoTerminal)
	assert.Contains(t, err.Error(), "the handle is invalid", "what the system said comes with it")
}

// A console this process cannot read the mode of is not one to type a secret
// into: echo could not be turned off, so the question is not asked at all.
func TestAConsoleWhoseEchoCannotBeTurnedOffIsNotAskedOn(t *testing.T) {
	t.Run("the mode cannot be read", func(t *testing.T) {
		con := &fakeConsole{modeErr: errNotAConsole, line: "secret\r\n"}
		con.install(t)
		_, err := ReadTTYLine("passphrase: ", true)
		require.ErrorIs(t, err, ErrNoTerminal)
		assert.Empty(t, con.written, "nothing was asked, so nothing could be typed in the clear")
	})

	t.Run("the mode cannot be set", func(t *testing.T) {
		con := &fakeConsole{setErr: errRefused, line: "secret\r\n"}
		con.install(t)
		_, err := ReadTTYLine("passphrase: ", true)
		require.Error(t, err)
		assert.Empty(t, con.written, "nothing was asked")
	})
}

// A question that is not a secret is asked with the console left alone: there
// is nothing to hide, and a confirmation the user cannot see themselves type
// would be worse than one they can.
func TestAQuestionThatIsNotASecretLeavesTheConsoleAsItIs(t *testing.T) {
	con := &fakeConsole{mode: windows.ENABLE_ECHO_INPUT, line: "yes\r\n"}
	con.install(t)

	answer, err := ReadTTYLine("are you sure? ", false)

	require.NoError(t, err)
	assert.Equal(t, "yes", answer)
	assert.Empty(t, con.modes, "the console's mode was never touched")
	assert.Equal(t, []string{"are you sure? ", "<read>"}, con.written,
		"and no newline of ours, since the console echoed the one that was typed")
}

// A question that could not be put on the console is not answered from it: the
// input is never read, so nothing typed before the failure — into a console
// whose echo is off and which is showing no prompt — can be taken for an answer.
func TestAQuestionThatCouldNotBeWrittenIsNotRead(t *testing.T) {
	con := &fakeConsole{mode: windows.ENABLE_ECHO_INPUT, line: "hunter2\r\n", writeErr: errRefused}
	con.install(t)

	_, err := ReadTTYLine("Enter passphrase for id_ed25519: ", true)

	require.ErrorIs(t, err, errRefused, "the failure to ask is what the caller is told")
	assert.NotContains(t, con.written, "<read>", "nothing was asked, so nothing is read as an answer")
	assert.True(t, con.closed, "the console is let go of all the same")
}

// A console that reported trouble is only trouble when nothing came back with
// it: the terminator the user pressed is an answer, and an answer that arrived
// is not thrown away because the read that carried it also complained.
func TestAFailedReadIsReportedOnlyWhenNothingCameBackWithIt(t *testing.T) {
	t.Run("nothing came back", func(t *testing.T) {
		con := &fakeConsole{mode: windows.ENABLE_ECHO_INPUT, readErr: errNotAConsole}
		con.install(t)

		_, err := ReadTTYLine("passphrase: ", true)

		assert.ErrorIs(t, err, errNotAConsole,
			"nothing was typed and the console said why, so that is what the caller gets")
	})

	t.Run("a line came back anyway", func(t *testing.T) {
		con := &fakeConsole{mode: windows.ENABLE_ECHO_INPUT, line: "hunter2\r\n", readErr: errNotAConsole}
		con.install(t)

		answer, err := ReadTTYLine("passphrase: ", true)

		require.NoError(t, err, "what the user typed arrived, so there is nothing to report")
		assert.Equal(t, "hunter2", answer)
	})
}

// F29: this prompter is offered wherever it might work, and says so plainly.
// What it needs is a console rather than a program to install, so its absence
// is Prompt's answer rather than a reason never to reach for it.
func TestTheConsolePrompterIsAlwaysOnOffer(t *testing.T) {
	assert.True(t, TTYPrompter{}.Available(t.Context()),
		"there is nothing to install for it, so it is always worth asking")
	assert.Equal(t, "the terminal", TTYPrompter{}.Name(),
		"named as the place the user is asked, in the same words as everywhere else")
}

// The prompter asks for the key it was given, on the console, with echo off.
func TestThePrompterAsksForTheKeyItWasGiven(t *testing.T) {
	con := &fakeConsole{mode: windows.ENABLE_ECHO_INPUT, line: "hunter2\r\n"}
	con.install(t)

	answer, err := TTYPrompter{}.Prompt(t.Context(), "id_ed25519")

	require.NoError(t, err)
	assert.Equal(t, "hunter2", answer)
	assert.Contains(t, con.written, "Enter passphrase for id_ed25519: ",
		"the question names the key being asked about")
}
