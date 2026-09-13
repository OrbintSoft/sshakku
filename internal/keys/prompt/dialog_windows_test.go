//go:build windows

package prompt

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

// What this prompter says about itself needs no window and no screen, so it is
// asked here rather than beside the tests that draw one.
func TestTheBoxNeedsNothingInstalled(t *testing.T) {
	assert.True(t, NativePrompter{}.Available(t.Context()),
		"SSHakku draws this box itself, so there is never anything missing to draw it with")
	assert.Equal(t, "native", NativePrompter{}.Name(),
		"the name is what gui_prompter calls it, so a message about it names something the user can write")
}

// errNoModuleHere is the failure these tests hand their seam, standing for a
// real one this program's own module cannot be made to produce.
var errNoModuleHere = errors.New("no module here")

// withNoModuleToBuildAgainst makes the one call everything drawn here is
// created against refuse, for the length of the (sub)test.
func withNoModuleToBuildAgainst(t *testing.T) {
	t.Helper()
	restore := getModuleHandleEx
	t.Cleanup(func() { getModuleHandleEx = restore })
	getModuleHandleEx = func(uint32, *uint16, *windows.Handle) error { return errNoModuleHere }
}

// F37 and F29: with nothing to create a window against, no box is drawn and
// the caller is told — at each of the three places that would otherwise carry
// on and build one on a handle of zero.
//
// This is the failure that cannot be arranged any other way: the module is the
// running executable, and it is always there. What is worth holding is that
// none of the three treats "no module" as something to continue past, because a
// window built on nothing is a window nobody can answer — and this program
// reads a box that was never answered as the user declining to give the
// passphrase.
func TestWithNothingToBuildAWindowAgainstNoBoxIsDrawn(t *testing.T) {
	t.Run("asking for the module itself", func(t *testing.T) {
		withNoModuleToBuildAgainst(t)

		_, err := moduleHandle()

		require.ErrorIs(t, err, errNoWindow, "the caller is told there is no box, not handed a zero")
	})

	t.Run("registering the class a window is drawn from", func(t *testing.T) {
		withNoModuleToBuildAgainst(t)
		// The registration happens once for the process and has already
		// happened; a fresh one is put in its place so this can reach the
		// failure, and the original — which succeeded — is restored after, so
		// nothing later re-registers a class that is already there.
		wasOnce, wasErr := classOnce, errClass
		classOnce, errClass = new(sync.Once), nil
		t.Cleanup(func() { classOnce, errClass = wasOnce, wasErr })

		err := registerWindowClass()

		require.ErrorIs(t, err, errNoWindow)
	})

	t.Run("putting the window on the screen", func(t *testing.T) {
		withNoModuleToBuildAgainst(t)
		w := &passphraseWindow{}

		_, err := w.create()

		require.ErrorIs(t, err, errNoWindow)
	})
}

// The screen's dpi decides how big the box is drawn. Where the system is too
// old to be asked, or answers nothing, the measurement falls back to the 96
// every number in the documentation assumes — rather than to a zero, which
// would scale every size in the box to nothing.
func TestABoxIsMeasuredAgainstNinetySixWhereTheScreenCannotBeAsked(t *testing.T) {
	restore := screenDPI
	t.Cleanup(func() { screenDPI = restore })

	t.Run("a system too old to be asked", func(t *testing.T) {
		screenDPI = func() (uintptr, bool) { return 0, false }
		assert.Equal(t, uint32(96), systemDPI())
	})

	t.Run("a system that answers nothing", func(t *testing.T) {
		screenDPI = func() (uintptr, bool) { return 0, true }
		assert.Equal(t, uint32(96), systemDPI(),
			"a dpi of zero would scale every size in the box to nothing")
	})

	t.Run("a screen that answers", func(t *testing.T) {
		screenDPI = func() (uintptr, bool) { return 192, true }
		assert.Equal(t, uint32(192), systemDPI(), "and a real answer is used as it is")
	})
}

// F37 and F38: a box the system refuses to put on the screen is reported as a
// failure to ask, and never as the user having declined.
//
// The difference is the whole of why this is worth a test. Closing the box is a
// decision, and this program acts on it: it stops asking about further keys for
// the rest of the login. A box that was never drawn is not that decision, and
// reading it as one would quietly give up on every remaining key because of a
// fault the user never saw.
//
// No window appears while this runs. The class the window would be drawn from
// is pointed at a name nothing ever registered, so the system refuses at
// creation — which is the one way to reach this without waiting for a machine
// that genuinely cannot draw.
func TestABoxTheSystemWouldNotDrawIsAFailureToAskAndNotARefusalToAnswer(t *testing.T) {
	// Registered first, under its own name, so that swapping the name below
	// reaches the creation and not the registration. A class registered under
	// the bogus name would be a class that exists, and the window would be
	// drawn from it — which is a real box on somebody's screen with nobody to
	// answer it.
	require.NoError(t, registerWindowClass())
	restore := className
	t.Cleanup(func() { className = restore })
	className = windows.StringToUTF16Ptr("SSHakkuNoSuchWindowClass")

	_, err := NativePrompter{Timeout: 5 * time.Second}.Prompt(t.Context(), "id_ed25519")

	require.Error(t, err, "there was no box, so there is no answer and no decision")
	assert.ErrorIs(t, err, errNoWindow, "what went wrong is that no box could be opened")
	assert.NotErrorIs(t, err, ErrCanceled,
		"a box that was never drawn is not the user closing one, which would stop this login asking about anything else")
}

// The same, met one step earlier: the class itself could not be registered, so
// there was never anything to draw a window from. It reaches the caller as the
// same kind of answer, because to the person in front of the shell it is.
func TestAClassThatCouldNotBeRegisteredIsAlsoAFailureToAsk(t *testing.T) {
	wasErr := errClass
	errClass = fmt.Errorf("%w: %w", errNoWindow, errNoModuleHere)
	t.Cleanup(func() { errClass = wasErr })

	_, err := NativePrompter{Timeout: 5 * time.Second}.Prompt(t.Context(), "id_ed25519")

	require.ErrorIs(t, err, errNoWindow)
	assert.NotErrorIs(t, err, ErrCanceled)
}
