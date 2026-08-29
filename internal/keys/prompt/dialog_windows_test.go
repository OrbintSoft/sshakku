//go:build windows

package prompt

import (
	"context"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

// These drive the real window. Nothing about it is stubbed: it is created,
// shown and destroyed by the system exactly as it is in front of a person, and
// what a person's keystrokes would deliver is delivered here as the messages
// they turn into. That is the only way to ask this window whether it works —
// a test that waited for somebody to type would hang a run instead of failing
// it, and a test that replaced the window would be asking itself.
//
// A window does flash on the screen of whoever runs these.

const (
	wmSetText = 0x000C
	bmClick   = 0x00F5
)

var (
	procFindWindow   = user32.NewProc("FindWindowW")
	procFindWindowEx = user32.NewProc("FindWindowExW")
)

// waitForBox waits for the passphrase window to be ready to be answered — which
// is the window *and* the field in it, since a frame with nothing in it yet is
// not something anybody could type into.
func waitForBox(t *testing.T) windows.HWND {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		h, _, _ := procFindWindow.Call(uintptr(unsafe.Pointer(className)), 0)
		if h != 0 && findEdit(windows.HWND(h)) != 0 {
			return windows.HWND(h)
		}
		time.Sleep(20 * time.Millisecond)
	}
	require.FailNow(t, "the passphrase window never appeared")
	return 0
}

// findEdit returns the box's masked input field, or zero while there is none.
func findEdit(box windows.HWND) windows.HWND {
	edit := windows.StringToUTF16Ptr("EDIT")
	h, _, _ := procFindWindowEx.Call(uintptr(box), 0, uintptr(unsafe.Pointer(edit)), 0)
	return windows.HWND(h)
}

// editOf returns the box's masked input field.
func editOf(t *testing.T, box windows.HWND) windows.HWND {
	t.Helper()
	h := findEdit(box)
	require.NotZero(t, h, "the box must hold a field to type into")
	return h
}

// typeInto puts text in the field the way typing does, and presses the button
// with the given id the way clicking it does.
func typeInto(t *testing.T, box windows.HWND, text string, button uintptr) {
	t.Helper()
	if text != "" {
		s := windows.StringToUTF16Ptr(text)
		_, _, _ = procSendMessage.Call(uintptr(editOf(t, box)), wmSetText, 0, uintptr(unsafe.Pointer(s)))
	}
	_, _, _ = procSendMessage.Call(uintptr(box), wmCommand, button, 0)
}

func TestTheBoxHandsBackWhatWasTypedInIt(t *testing.T) {
	answers := make(chan string, 1)
	errs := make(chan error, 1)
	go func() {
		pass, err := NativePrompter{}.Prompt(context.WithoutCancel(t.Context()), "id_ed25519")
		answers <- pass
		errs <- err
	}()

	box := waitForBox(t)
	// A trailing space, because a passphrase may end in one and nothing here is
	// entitled to tidy it away.
	typeInto(t, box, "correct horse ", idOK)

	require.NoError(t, <-errs, "a box the user answered must hand the answer back")
	assert.Equal(t, "correct horse ", <-answers, "and it must be what they typed, to the last character")
}

func TestABoxDismissedIsADecisionAndNotAnAnswer(t *testing.T) {
	errs := make(chan error, 1)
	go func() {
		_, err := NativePrompter{}.Prompt(context.WithoutCancel(t.Context()), "id_ed25519")
		errs <- err
	}()

	box := waitForBox(t)
	typeInto(t, box, "typed but not submitted", idCancel)

	assert.ErrorIs(t, <-errs, ErrCanceled,
		"closing the box is a decision, and must be passed on as one rather than as a failure")
}

func TestTheWindowsOwnCloseIsADismissalToo(t *testing.T) {
	errs := make(chan error, 1)
	go func() {
		_, err := NativePrompter{}.Prompt(context.WithoutCancel(t.Context()), "id_ed25519")
		errs <- err
	}()

	box := waitForBox(t)
	_, _, _ = procSendMessage.Call(uintptr(box), wmClose, 0, 0)

	assert.ErrorIs(t, <-errs, ErrCanceled,
		"the frame's own close button is the same gesture as Cancel, and means the same thing")
}

func TestABoxNobodyAnsweredIsNotADismissal(t *testing.T) {
	// The distinction the whole design turns on: a budget that ran out is not
	// somebody deciding. Read as a dismissal it would end the asking for the
	// rest of the login with nobody having been asked anything.
	_, err := NativePrompter{Timeout: 300 * time.Millisecond}.Prompt(context.WithoutCancel(t.Context()), "id_ed25519")

	require.Error(t, err, "a box nobody answered in the time allowed is a failure to ask")
	assert.ErrorIs(t, err, errBudgetRanOut, "and it says so as something a caller can match on")
	assert.NotErrorIs(t, err, ErrCanceled, "it is emphatically not the user's decision")
}

func TestTheBoxNeedsNothingInstalled(t *testing.T) {
	assert.True(t, NativePrompter{}.Available(t.Context()),
		"SSHakku draws this box itself, so there is never anything missing to draw it with")
	assert.Equal(t, "native", NativePrompter{}.Name(),
		"the name is what gui_prompter calls it, so a message about it names something the user can write")
}
