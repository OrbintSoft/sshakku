//go:build windows

package prompt

import (
	"context"
	"errors"
)

// ErrNoTerminal is returned when there is no controlling terminal to prompt
// on. A headless session with no GUI and no tty is a normal, expected
// deployment, not a failure — callers treat it as "could not prompt this
// round" rather than logging or surfacing it as an error.
var ErrNoTerminal = errors.New("no controlling terminal available")

// errNoConsolePrompt is what ReadTTYLine reports here. It is deliberately not
// ErrNoTerminal: this system has a console to ask on (CONIN$/CONOUT$, and a
// console mode that can turn echo off), so answering "there is nobody to ask"
// would report a missing implementation as an empty room, and callers treat an
// empty room as normal and stay silent about it.
var errNoConsolePrompt = errors.New("prompting on the console is not implemented on windows")

// ReadTTYLine reports errNoConsolePrompt.
func ReadTTYLine(string, bool) (string, error) { return "", errNoConsolePrompt }

// TTYPrompter prompts for a passphrase on the console. Available reports false
// here, which is what keeps it from being offered as the fallback that always
// works: on this platform it cannot yet ask anything.
type TTYPrompter struct{}

// Prompt reports errNoConsolePrompt.
func (TTYPrompter) Prompt(context.Context, string) (string, error) { return "", errNoConsolePrompt }

// Available reports false: there is no console prompt to fall back to here.
func (TTYPrompter) Available(context.Context) bool { return false }

// Name is what to call this prompter in a message: the place the user would be
// asked, not a program they could go and install. It is the same word here as
// everywhere else — a message that named this system's console while the rest
// of SSHakku says "the terminal" would read as a second, different place.
func (TTYPrompter) Name() string { return "the terminal" }

var _ Prompter = TTYPrompter{}
