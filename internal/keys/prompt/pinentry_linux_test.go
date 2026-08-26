//go:build linux

package prompt

import (
	"bufio"
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The failures these tests hand their seams. Each stands for a real one the
// code under test cannot be made to produce on demand.
var (
	errStdinAlreadySet  = errors.New("stdin already set")
	errStdoutAlreadySet = errors.New("stdout already set")
)

// fakePinentry is the path to a program that speaks the protocol for real, so
// these tests drive a process and a pipe rather than a stand-in for one.
const fakePinentry = "../../../test/fakes/pinentry.sh"

func TestPinentryPrompt(t *testing.T) {
	t.Run("returns what was typed", func(t *testing.T) {
		t.Setenv("SSHAKKU_TEST_PINENTRY_PIN", "correct horse")

		pass, err := PinentryPrompter{Bin: fakePinentry}.Prompt(t.Context(), "id_rsa")
		require.NoError(t, err, "a dialog the user answered must hand the answer back")
		assert.Equal(t, "correct horse", pass, "and it must be what they typed")
	})

	t.Run("a dismissed dialog is ErrCanceled", func(t *testing.T) {
		t.Setenv("SSHAKKU_TEST_PINENTRY_CANCEL", "1")

		_, err := PinentryPrompter{Bin: fakePinentry}.Prompt(t.Context(), "id_rsa")
		assert.ErrorIs(t, err, ErrCanceled,
			"closing a dialog is an answer, and must be passed on as one rather than as a failure")
	})

	t.Run("status lines and comments are not the answer", func(t *testing.T) {
		t.Setenv("SSHAKKU_TEST_PINENTRY_NOISE", "1")
		t.Setenv("SSHAKKU_TEST_PINENTRY_PIN", "the-passphrase")

		pass, err := PinentryPrompter{Bin: fakePinentry}.Prompt(t.Context(), "id_rsa")
		require.NoError(t, err, "a dialog the user answered must hand the answer back")
		assert.Equal(t, "the-passphrase", pass,
			"pinentry may say things about itself at any point, and none of them is the passphrase")
	})

	t.Run("percent-escaped bytes come back as they were typed", func(t *testing.T) {
		t.Setenv("SSHAKKU_TEST_PINENTRY_PIN", "a%25b%0Ac")

		pass, err := PinentryPrompter{Bin: fakePinentry}.Prompt(t.Context(), "id_rsa")
		require.NoError(t, err, "a dialog the user answered must hand the answer back")
		assert.Equal(t, "a%b\nc", pass,
			"the protocol escapes bytes on the wire, and a passphrase left escaped is not the one that was typed")
	})

	t.Run("an unanswered dialog does not strand the caller", func(t *testing.T) {
		t.Setenv("SSHAKKU_TEST_PINENTRY_HANG", "1")

		start := time.Now()
		_, err := PinentryPrompter{Bin: fakePinentry, Timeout: 300 * time.Millisecond}.Prompt(t.Context(), "id_rsa")
		assert.Error(t, err, "a dialog that never answered must end as an error the caller can fall back from")
		assert.Less(t, time.Since(start), 5*time.Second,
			"and within the bound it was given: something is waiting on this, and it must not wait for ever")
	})

	t.Run("a pinentry that cannot be started is an error, not a hang", func(t *testing.T) {
		_, err := PinentryPrompter{Bin: "/nonexistent/pinentry"}.Prompt(t.Context(), "id_rsa")
		require.Error(t, err, "a dialog that cannot be started cannot ask, and that must be said")
		assert.NotErrorIs(t, err, ErrCanceled,
			"but not as a dismissal: nobody was there to dismiss anything, and the question can still go elsewhere")
	})
}

// TestPinentryAvailable covers what "there is a pinentry to ask in" has to mean
// for the chain that reads it: not that a program by that name exists, but that
// what it runs can put a dialog on the screen the user is sitting at.
func TestPinentryAvailable(t *testing.T) {
	installed := func(string) (string, error) { return "/usr/bin/pinentry", nil }

	t.Run("not installed", func(t *testing.T) {
		p := PinentryPrompter{lookPath: func(string) (string, error) { return "", errNotFound }}
		assert.False(t, p.Available(t.Context()), "a program that is not installed cannot put a dialog on any screen")
	})

	t.Run("a build that draws on a screen", func(t *testing.T) {
		t.Setenv("SSHAKKU_TEST_PINENTRY_FLAVOR", "gtk2:curses")

		p := PinentryPrompter{Bin: fakePinentry, lookPath: installed}
		assert.True(t, p.Available(t.Context()),
			"a pinentry that draws with GTK is a dialog: the console it also falls back to is not what it leads with")
	})

	t.Run("a build that draws on a terminal is not a dialog", func(t *testing.T) {
		for _, flavor := range []string{"curses", "tty"} {
			t.Setenv("SSHAKKU_TEST_PINENTRY_FLAVOR", flavor)

			p := PinentryPrompter{Bin: fakePinentry, lookPath: installed}
			assert.Falsef(t, p.Available(t.Context()),
				"the %s build draws on a terminal, so counting it as a dialog would take the question from one that "+
					"can be drawn and leave it with nowhere to appear", flavor)
		}
	})

	t.Run("an answer nobody here understands counts as a dialog", func(t *testing.T) {
		t.Setenv("SSHAKKU_TEST_PINENTRY_FLAVOR", "a-toolkit-nobody-has-written-yet")

		p := PinentryPrompter{Bin: fakePinentry, lookPath: installed}
		assert.True(t, p.Available(t.Context()),
			"a build this code has never heard of counts as a dialog: passing over one that works is the worse mistake")
	})

	t.Run("a pinentry that cannot be asked counts as a dialog", func(t *testing.T) {
		p := PinentryPrompter{Bin: "/nonexistent/pinentry", lookPath: installed}
		assert.True(t, p.Available(t.Context()),
			"a pinentry too old to say what it draws with is not one that cannot draw")
	})

	t.Run("names the program a message would send the user to look for", func(t *testing.T) {
		assert.Equal(t, pinentryBin, Name(PinentryPrompter{}),
			"the name in a message is what the user goes looking for")
	})

	t.Run("says both of the things it may be", func(t *testing.T) {
		why := Unavailable(PinentryPrompter{})
		assert.Contains(t, why, "not installed", "it may not be there")
		assert.Contains(t, why, "terminal",
			"or it may be a build that draws on a terminal; a user told only the first would go and install what they have")
	})

	t.Run("a pinentry that never answers does not strand the caller", func(t *testing.T) {
		t.Setenv("SSHAKKU_TEST_PINENTRY_HANG", "1")

		start := time.Now()
		PinentryPrompter{Bin: fakePinentry, lookPath: installed, ProbeTimeout: 300 * time.Millisecond}.Available(t.Context())
		assert.Less(t, time.Since(start), 5*time.Second,
			"asking pinentry about itself waits on no person, and must be bounded like any other command")
	})
}

// TestAssuanErrorDescribesWhatFailed covers the errors that are not a
// cancellation: what pinentry said has to survive as far as the log, or a
// misconfigured dialog is indistinguishable from one nobody answered.
func TestAssuanErrorDescribesWhatFailed(t *testing.T) {
	t.Run("a cancellation, whichever component reports it", func(t *testing.T) {
		for _, line := range []string{"83886179 Operation cancelled", "83886180 Operation fully cancelled"} {
			assert.ErrorIsf(t, assuanError(line), ErrCanceled,
				"%q is the user closing the dialog, whichever component reports it", line)
		}
	})

	t.Run("anything else keeps its description", func(t *testing.T) {
		err := assuanError("83886254 No pinentry")
		require.Error(t, err, "something that is not a cancellation is a failure")
		assert.Contains(t, err.Error(), "No pinentry",
			"and what pinentry said must survive as far as the log, or a misconfigured dialog looks like an unanswered one")
	})

	t.Run("a line no number can be read from is still an error", func(t *testing.T) {
		assert.Error(t, assuanError("something went wrong"),
			"a line nothing can be read from is still pinentry refusing, not pinentry answering")
	})

	t.Run("a number with nothing said about it still says which number", func(t *testing.T) {
		err := assuanError("83886254")
		require.Error(t, err, "a bare code is still a refusal")
		assert.Contains(t, err.Error(), "83886254", "and the number must be kept: it is all there is to go on")
	})
}

// TestPinentryConversationEndsWhenItCannotBeWrittenTo covers a pinentry that
// goes away mid-conversation. What is being asked of it is a passphrase, so
// failing to say so has to end as an error the caller can fall back from,
// rather than as an empty answer that would look like one the user gave.
func TestPinentryConversationEndsWhenItCannotBeWrittenTo(t *testing.T) {
	conv := &assuanConv{w: closedPipe{}, r: bufio.NewReader(strings.NewReader("OK Pleased to meet you\n"))}

	pass, err := conv.getpin("id_rsa")
	assert.Error(t, err, "a pinentry that has gone away cannot be asked, and that must end as an error")
	assert.Empty(t, pass, "and nothing may come back that would read as an answer the user gave")
}

// closedPipe is the write end of a pipe whose reader has gone, as a pinentry
// that has exited leaves behind.
type closedPipe struct{}

func (closedPipe) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

// TestPinentryConversationWithNoPipesToTalkOver covers what happens when the
// conversation cannot be opened at all: the caller is told, and no dialog is
// left running with nobody reading it. Neither failure can be produced from
// outside — exec.Cmd refuses these pipes only for a command that is already
// wired up or already started — so the pipes are taken through a seam the way
// the terminal is elsewhere in this package.
func TestPinentryConversationWithNoPipesToTalkOver(t *testing.T) {
	saved := func(t *testing.T) {
		t.Helper()
		in, out := stdinPipe, stdoutPipe
		t.Cleanup(func() { stdinPipe, stdoutPipe = in, out })
	}

	t.Run("nothing to write to", func(t *testing.T) {
		saved(t)
		stdinPipe = func(*exec.Cmd) (io.WriteCloser, error) { return nil, errStdinAlreadySet }
		_, err := PinentryPrompter{Bin: fakePinentry}.Prompt(t.Context(), "id_rsa")
		assert.Error(t, err, "with no way to put the question, the caller must be told rather than left waiting")
	})

	t.Run("nothing to read from", func(t *testing.T) {
		saved(t)
		stdoutPipe = func(*exec.Cmd) (io.ReadCloser, error) { return nil, errStdoutAlreadySet }
		_, err := PinentryPrompter{Bin: fakePinentry}.Prompt(t.Context(), "id_rsa")
		assert.Error(t, err, "with no way to hear the answer, no dialog may be left running with nobody reading it")
	})
}

// TestPinentryAvailableDefaultLookPath covers what a caller that supplies no
// lookup gets: the real PATH. Naming a program nobody has is what makes the
// answer the lookup's rather than this machine's.
func TestPinentryAvailableDefaultLookPath(t *testing.T) {
	p := PinentryPrompter{Bin: "sshakku-no-such-pinentry"}
	assert.False(t, p.Available(t.Context()),
		"with no lookup supplied the real PATH is what answers, and a program that is not on it "+
			"cannot put a dialog anywhere")
}

// TestAssuanGreetingNeverArrives covers what each half of the conversation
// makes of a pinentry that starts, says nothing and exits — one too old for the
// protocol, or a name on PATH that turned out to be something else. Both halves
// fail either way; what is judged here is that they fail saying so, because
// "the greeting never came" and "the pipe broke while asking" send whoever
// reads the log to different places.
func TestAssuanGreetingNeverArrives(t *testing.T) {
	silent := func() *assuanConv {
		return &assuanConv{w: io.Discard, r: bufio.NewReader(strings.NewReader(""))}
	}

	t.Run("asking for a passphrase", func(t *testing.T) {
		_, err := silent().getpin("id_rsa")
		require.Error(t, err, "a dialog that never spoke cannot have taken an answer")
		assert.Contains(t, err.Error(), "greeting",
			"and the line has to say the program never announced itself, or it reads as a dialog that broke mid-question")
	})

	t.Run("asking what it draws with", func(t *testing.T) {
		_, err := silent().flavor()
		require.Error(t, err, "a pinentry that never spoke said nothing about what it draws with either")
		assert.Contains(t, err.Error(), "greeting", "and the line has to say which of the two it was")
	})
}
