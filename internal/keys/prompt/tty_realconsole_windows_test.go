//go:build windows

package prompt

import (
	"testing"

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
