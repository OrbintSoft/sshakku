//go:build unix

package keys

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

// ErrNoTerminal is returned when there is no controlling terminal to prompt
// on. A headless session with no GUI and no tty is a normal, expected
// deployment, not a failure — callers treat it as "could not prompt this
// round" rather than logging or surfacing it as an error.
var ErrNoTerminal = errors.New("no controlling terminal available")

// Seams over the controlling terminal and its echo toggle. Production opens the
// real /dev/tty and drives the real termios ioctls; tests point them at a
// socketpair and stubbed ioctls so ReadTTYLine and disableEcho are exercisable
// without a live terminal.
var (
	openTTY    = func() (*os.File, error) { return os.OpenFile("/dev/tty", os.O_RDWR, 0) }
	getTermios = unix.IoctlGetTermios
	setTermios = unix.IoctlSetTermios
)

// ReadTTYLine writes prompt to /dev/tty and reads one line from it, optionally
// with echo disabled. It opens /dev/tty directly rather than reading stdin, so
// it works even when the caller has been detached from its original stdin;
// with no controlling terminal at all the open fails immediately — it never
// blocks waiting for one to appear — reported as ErrNoTerminal. A user who
// closes the input instead of answering gets ErrPromptCanceled.
func ReadTTYLine(prompt string, secret bool) (string, error) {
	f, err := openTTY()
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrNoTerminal, err)
	}
	defer func() { _ = f.Close() }()

	// Echo goes off before the prompt is written, never after: the terminal
	// echoes each character as it arrives, so anything typed (or pasted, or
	// piped in) between the two would be printed in the clear. Seeing the
	// prompt is then also the user's guarantee that echo is already off.
	if secret {
		restore, err := disableEcho(f)
		if err != nil {
			return "", err
		}
		defer restore()
	}

	if _, err := fmt.Fprint(f, prompt); err != nil {
		return "", err
	}

	line, readErr := bufio.NewReader(f).ReadString('\n')
	if secret {
		// The newline the user pressed was not echoed; emit one so later
		// output does not run onto the prompt line.
		_, _ = fmt.Fprintln(f)
	}
	if errors.Is(readErr, io.EOF) && line == "" {
		// Closing the input instead of answering — Ctrl-D — is how a question is
		// turned down on a terminal, the same gesture as closing a dialog. It is
		// the user's decision, not a failure to read from the terminal.
		return "", ErrPromptCanceled
	}
	if readErr != nil && line == "" {
		return "", readErr
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// disableEcho turns off terminal echo on f, returning a function that restores
// the previous terminal state.
func disableEcho(f *os.File) (func(), error) {
	fd := int(f.Fd())
	old, err := getTermios(fd, tcGetTermiosReq)
	if err != nil {
		return nil, err
	}
	raw := *old
	raw.Lflag &^= unix.ECHO
	if err := setTermios(fd, tcSetTermiosReq, &raw); err != nil {
		return nil, err
	}
	return func() { _ = setTermios(fd, tcSetTermiosReq, old) }, nil
}

// TTYPrompter prompts for a passphrase directly on /dev/tty — the fallback
// used when no graphical prompter is available. Unlike KDialogPrompter it
// needs no external binary, so Available always reports true; a missing
// controlling terminal surfaces as ErrNoTerminal from Prompt instead, which
// the loader treats as "could not prompt this round" rather than an error.
// Closing the input at the prompt is the terminal's way of dismissing the
// question and is reported as such.
type TTYPrompter struct{}

// Prompt asks for keyname's passphrase on /dev/tty, with echo disabled.
func (TTYPrompter) Prompt(keyname string) (string, error) {
	return ReadTTYLine("Enter passphrase for "+keyname+": ", true)
}

// Available always reports true: TTYPrompter needs no external binary, only a
// controlling terminal, whose absence is reported by Prompt instead.
func (TTYPrompter) Available() bool { return true }

// Name is what to call this prompter in a message. It is not a program the user
// could go and install, so it is named the way they would recognise it: the
// place they were asked.
func (TTYPrompter) Name() string { return "the terminal" }

var _ Prompter = TTYPrompter{}
