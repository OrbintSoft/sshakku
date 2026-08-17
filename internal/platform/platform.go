// Package platform is how this build says that something is not implemented on
// the system it is running on.
//
// A mechanism one operating system has and another does not is not a fault of
// the session that met it. A login shell that cannot be given an ssh-agent
// should still open, and open silently; a diagnostic report should name the
// piece that is absent rather than recommend the thing that would have worked
// elsewhere. Both of those need to tell "there is nothing here to do this
// with" apart from "this went wrong", and a message cannot be told apart from
// anything — so what says it is a sentinel, and every stub that reports itself
// absent wraps this one.
//
// It is deliberately not errors.ErrUnsupported. That one is also returned by
// the standard library when a real operation is refused — a filesystem that
// cannot do what was asked, say — and a session must not treat a genuine
// refusal as a piece this project has yet to write.
package platform

import (
	"errors"
	"fmt"
	"runtime"
)

// ErrUnimplemented is what a mechanism this build has no implementation of
// here reports, wrapped so a caller can recognise it with errors.Is.
//
// It names the operating system because that is the whole of what is being
// said: the same call on another one may well be implemented, and somebody
// reading the message needs to know which machine it is about.
var ErrUnimplemented = errors.New("not implemented on " + runtime.GOOS)

// Unimplemented reports that the named thing is not implemented here.
//
// what names the mechanism from the caller's point of view — "keeping an
// ssh-agent on a fixed endpoint", not the function that would have done it —
// because the message is read by somebody deciding what they can do about it.
func Unimplemented(what string) error {
	return fmt.Errorf("%s: %w", what, ErrUnimplemented)
}
