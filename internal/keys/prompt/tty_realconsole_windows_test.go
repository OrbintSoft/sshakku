//go:build windows

package prompt

import (
	"errors"
	"testing"
	"unicode/utf16"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"golang.org/x/sys/windows"
)

// The console half of this package for real — the functions the seams stand in
// for elsewhere — driven without a console anybody has to type into. Nothing
// here may wait on a person: a test that blocked for a keypress would hang the
// run instead of failing it, so every call below is one the system answers on
// its own, and the handle they are given is one that cannot serve a read.

// TestOpeningTheConsoleAnswersAtOnceAndAlwaysHandsBackACloser: the caller defers
// the closer before it knows whether there was a console, so one is returned
// either way. Which way it goes depends on the session this runs in — a console
// is a property of the session, not of this code — and both are correct; what
// is not is waiting for one to appear.
func TestOpeningTheConsoleAnswersAtOnceAndAlwaysHandsBackACloser(t *testing.T) {
	con, closeConsole, err := openRealConsole()
	require.NotNil(t, closeConsole, "a closer comes back even when there was nothing to open")
	defer closeConsole()

	if err != nil {
		assert.Equal(t, console{}, con, "a console that could not be opened is no console at all")
		return
	}
	assert.NotEqual(t, windows.InvalidHandle, con.in, "the half a person types into")
	assert.NotEqual(t, windows.InvalidHandle, con.out, "the half the question is written to")
}

// TestANameTheSystemCannotSpellIsRefusedBeforeAnythingIsOpened: a console half
// is asked for by name, and a name this system cannot be given is turned back
// where it is converted — not handed over as something shorter than it reads,
// which would name a different file.
func TestANameTheSystemCannotSpellIsRefusedBeforeAnythingIsOpened(t *testing.T) {
	handle, err := openConsoleFile("CONIN$\x00and more")
	require.Error(t, err)
	assert.Equal(t, windows.InvalidHandle, handle, "nothing was opened, so there is no handle to give back")
}

// TestReadingFromWhatIsNotAConsoleFailsRatherThanReturnsNothing: ReadTTYLine
// reads nothing at all as the user closing the input — their way of turning the
// question down — so a read that could not happen has to arrive as a failure
// and not as that gesture.
func TestReadingFromWhatIsNotAConsoleFailsRatherThanReturnsNothing(t *testing.T) {
	line, err := readRealConsole(windows.InvalidHandle)
	require.Error(t, err, "a read that cannot happen is not an empty line")
	assert.Empty(t, line)
}

// TestWritingNothingIsNotAWrite: an empty prompt, and the newline that is not
// needed when the console echoed the user's own, both arrive here as an empty
// string. It must not become a call carrying a pointer to no characters at all.
func TestWritingNothingIsNotAWrite(t *testing.T) {
	assert.NoError(t, writeRealConsole(windows.InvalidHandle, ""),
		"nothing to write is not a failure, and not a call either: this handle could not have served one")
}

// TestWritingToWhatIsNotAConsoleIsReported: the question has to be on the screen
// before it can be answered, so a write that did not happen is something the
// caller stops on rather than reads past.
func TestWritingToWhatIsNotAConsoleIsReported(t *testing.T) {
	assert.Error(t, writeRealConsole(windows.InvalidHandle, "Enter passphrase for id_ed25519: "))
}

// errNoConsoleHere is the failure these tests hand their seams, standing for a
// real one a session that has a console cannot be made to produce.
var errNoConsoleHere = errors.New("no console here")

// F29: a session with no console to ask on is told so, and neither half of one
// is left open behind the refusal.
//
// The two halves are opened separately and either can be the one that refuses.
// What must not happen is the second refusing while the first stays open: a
// process that failed to ask would then be holding a handle to the console it
// did not ask on, for as long as it lives.
func TestAConsoleThatWillNotOpenIsReportedAndNothingIsLeftHolding(t *testing.T) {
	t.Run("the half that is read from", func(t *testing.T) {
		restore := openConsoleHandle
		t.Cleanup(func() { openConsoleHandle = restore })
		openConsoleHandle = func(string) (windows.Handle, error) { return windows.InvalidHandle, errNoConsoleHere }

		_, closeConsole, err := openRealConsole()

		require.ErrorIs(t, err, errNoConsoleHere)
		require.NotNil(t, closeConsole, "a caller always has something to defer, even where nothing opened")
		closeConsole()
	})

	// The half that is written to refuses after the half that is read from has
	// already opened. The code gives the open one back before it returns; that
	// the handle really is released is the system's to know and not observable
	// from here, so what this holds is that the refusal reaches the caller
	// rather than being lost behind the half that worked.
	t.Run("the half that is written to", func(t *testing.T) {
		restore := openConsoleHandle
		t.Cleanup(func() { openConsoleHandle = restore })
		asked := make([]string, 0, 2)
		openConsoleHandle = func(name string) (windows.Handle, error) {
			asked = append(asked, name)
			if name == "CONOUT$" {
				return windows.InvalidHandle, errNoConsoleHere
			}
			return windows.Handle(0x1234), nil
		}

		_, _, err := openRealConsole()

		require.ErrorIs(t, err, errNoConsoleHere)
		assert.Equal(t, []string{"CONIN$", "CONOUT$"}, asked,
			"both halves were asked for, so this is the case where one opened and the other did not")
	})
}

// F29: a console that stops answering mid-question is reported rather than
// handed on as the passphrase somebody typed. An empty string is what an
// answer of nothing looks like, and only one of the two is an answer.
func TestAConsoleThatFailsMidReadIsNotAnAnswer(t *testing.T) {
	restore := readConsoleInto
	t.Cleanup(func() { readConsoleInto = restore })
	readConsoleInto = func(windows.Handle, *uint16, uint32, *uint32, *byte) error { return errNoConsoleHere }

	got, err := readRealConsole(windows.InvalidHandle)

	require.ErrorIs(t, err, errNoConsoleHere)
	assert.Empty(t, got, "nothing was read, and nothing is what is handed back — as an error, not as a passphrase")
}

// What a console hands over is UTF-16 with its terminator included, and what
// comes back is the line as typed. The read itself needs somebody at the
// keyboard, so the console's answer is handed over instead — what is being
// judged is the decoding of it, which is this program's own.
func TestALineIsReadBackTheWayTheConsoleHandsItOver(t *testing.T) {
	restore := readConsoleInto
	t.Cleanup(func() { readConsoleInto = restore })
	readConsoleInto = func(_ windows.Handle, buf *uint16, toread uint32, read *uint32, _ *byte) error {
		typed := utf16.Encode([]rune("a pässphrase\r\n"))
		require.LessOrEqual(t, len(typed), int(toread), "the fixture must fit the buffer the code offered")
		copy(unsafe.Slice(buf, toread), typed)
		*read = uint32(len(typed)) //nolint:gosec // G115 sees the length of a fixture written three lines above
		return nil
	}

	got, err := readRealConsole(windows.InvalidHandle)

	require.NoError(t, err)
	assert.Equal(t, "a pässphrase\r\n", got,
		"the terminator is kept: an answer of nothing has to be tellable from an input that was closed")
}
