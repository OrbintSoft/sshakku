//go:build unix

package prompt

import "errors"

// The failures these tests hand their seams. Each stands for a real one the
// code under test cannot be made to produce on demand.
var (
	errBoom     = errors.New("boom")
	errNotFound = errors.New("not found")
)

// notOnPathError is what a runner answers with when the program it was asked
// to run is not installed. It carries the name, because the whole point of the
// test is that the name in the message is the one the user would type into
// gui_prompter.
type notOnPathError struct{ name string }

func (e notOnPathError) Error() string {
	return "exec: \"" + e.name + "\": executable file not found in $PATH"
}
