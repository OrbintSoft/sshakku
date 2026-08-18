//go:build windows

package prompt

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

// ErrNoTerminal is returned when there is no console to prompt on. A session
// with no GUI and no console is a normal, expected deployment, not a failure —
// callers treat it as "could not prompt this round" rather than logging or
// surfacing it as an error.
var ErrNoTerminal = errors.New("no controlling terminal available")

// console is what a question needs: the buffer a person types into and the one
// the question is written to. They are two objects here rather than one device
// opened both ways, and both are reached by name — not through this process's
// standard input and output, which is what makes a prompt possible in the very
// process that was deliberately given no stdin.
type console struct{ in, out windows.Handle }

// Seams over the console and its echo toggle. Production opens the real one
// and drives the real console modes; tests stand in for both, so the order of
// what is done — echo off before the question is asked, restored after, and a
// closed input told from an empty answer — is exercisable without a console to
// type into.
var (
	openConsole    = openRealConsole
	consoleModeOf  = windows.GetConsoleMode
	setConsoleMode = windows.SetConsoleMode
	readConsole    = readRealConsole
	writeConsole   = writeRealConsole
)

// ReadTTYLine writes prompt to the console and reads one line back from it,
// optionally with echo disabled. Where there is no console the open fails
// immediately — it never blocks waiting for one to appear — reported as
// ErrNoTerminal. A user who closes the input instead of answering gets
// ErrCanceled.
func ReadTTYLine(prompt string, secret bool) (string, error) {
	con, closeConsole, err := openConsole()
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrNoTerminal, err)
	}
	defer closeConsole()

	// Echo goes off before the prompt is written, never after: the console
	// echoes each character as it arrives, so anything typed (or pasted)
	// between the two would be printed in the clear. Seeing the prompt is then
	// also the user's guarantee that echo is already off.
	if secret {
		restore, err := disableEcho(con.in)
		if err != nil {
			return "", err
		}
		defer restore()
	}

	if err := writeConsole(con.out, prompt); err != nil {
		return "", err
	}

	line, readErr := readConsole(con.in)
	if secret {
		// The newline the user pressed was not echoed; emit one so later
		// output does not run onto the prompt line.
		_ = writeConsole(con.out, "\r\n")
	}
	if readErr != nil && line == "" {
		return "", readErr
	}
	if line == "" {
		// Closing the input instead of answering — Ctrl-Z here, Ctrl-D
		// elsewhere — is how a question is turned down at a console, the same
		// gesture as closing a dialog. It is the user's decision, not a failure
		// to read: an answer of nothing at all still arrives as the line
		// terminator the user pressed.
		return "", ErrCanceled
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// disableEcho turns off echo on the console input, returning a function that
// puts the mode back as it was. Only that one flag is touched: line editing and
// the console's own handling of Ctrl-C are what a person expects while typing,
// and taking them away to hide a passphrase would take away more than echo.
func disableEcho(in windows.Handle) (func(), error) {
	var mode uint32
	if err := consoleModeOf(in, &mode); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoTerminal, err)
	}
	if err := setConsoleMode(in, mode&^windows.ENABLE_ECHO_INPUT); err != nil {
		return nil, err
	}
	return func() { _ = setConsoleMode(in, mode) }, nil
}

// openRealConsole opens this session's console by name and returns it with the
// function that closes it again.
func openRealConsole() (console, func(), error) {
	in, err := openConsoleFile("CONIN$")
	if err != nil {
		return console{}, func() {}, err
	}
	out, err := openConsoleFile("CONOUT$")
	if err != nil {
		_ = windows.CloseHandle(in)
		return console{}, func() {}, err
	}
	return console{in: in, out: out}, func() {
		_ = windows.CloseHandle(in)
		_ = windows.CloseHandle(out)
	}, nil
}

// openConsoleFile opens one half of the console. Both halves are shared: a
// console belongs to the session rather than to this process, and asking for it
// exclusively would fail wherever anything else already has it — which is
// everything, since the shell that started us is holding it too.
func openConsoleFile(name string) (windows.Handle, error) {
	wide, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return windows.InvalidHandle, err
	}
	return windows.CreateFile(wide,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil, windows.OPEN_EXISTING, 0, 0)
}

// readRealConsole reads one line as the console hands it over: UTF-16 units,
// terminator included, so an answer of nothing can be told from an input that
// was closed.
func readRealConsole(in windows.Handle) (string, error) {
	// A passphrase is one line typed by a person; this is far more than one and
	// is a bound rather than an expectation.
	buf := make([]uint16, 4096)
	var read uint32
	if err := windows.ReadConsole(in, &buf[0], uint32(len(buf)), &read, nil); err != nil {
		return "", err
	}
	return string(utf16.Decode(buf[:read])), nil
}

// writeRealConsole writes s to the console.
func writeRealConsole(out windows.Handle, s string) error {
	if s == "" {
		return nil
	}
	wide := utf16.Encode([]rune(s))
	var written uint32
	return windows.WriteConsole(out, &wide[0], uint32(len(wide)), &written, nil)
}

// TTYPrompter prompts for a passphrase on this session's console — the
// fallback used when no graphical prompter is available. It needs no external
// program, so Available always reports true; a session with no console at all
// surfaces as ErrNoTerminal from Prompt instead, which the loader treats as
// "could not prompt this round" rather than an error. Closing the input at the
// prompt is this platform's way of dismissing the question and is reported as
// such.
type TTYPrompter struct{}

// Prompt asks for keyname's passphrase on the console, with echo disabled.
func (TTYPrompter) Prompt(_ context.Context, keyname string) (string, error) {
	return ReadTTYLine("Enter passphrase for "+keyname+": ", true)
}

// Available always reports true: TTYPrompter needs no external program, only a
// console, whose absence is reported by Prompt instead.
func (TTYPrompter) Available(context.Context) bool { return true }

// Name is what to call this prompter in a message: the place the user would be
// asked, not a program they could go and install. It is the same word here as
// everywhere else — a message that named this system's console while the rest
// of SSHakku says "the terminal" would read as a second, different place.
func (TTYPrompter) Name() string { return "the terminal" }

var _ Prompter = TTYPrompter{}
