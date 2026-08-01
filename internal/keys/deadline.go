package keys

import (
	"errors"
	"fmt"
	"time"
)

// ErrTimedOut reports that something SSHakku was waiting on neither answered
// nor failed inside the budget it was given. Only the waiting has ended:
// whatever was being waited on may well still be going on.
var ErrTimedOut = errors.New("timed out")

// withDeadline runs call on a goroutine of its own and returns what it
// produced, or gives up on it once timeout has elapsed. what names the thing
// being waited on, so a session log says which one ran out of time.
//
// It exists for a call that cannot be interrupted — a library entered through
// cgo, where nothing in Go can reach in and stop it. Giving up is therefore not
// cancelling: the call goes on for as long as it likes and only the waiting
// stops, so the goroutine is expected to outlive this function. The channel is
// buffered for exactly that reason — when the call does finally return, it must
// be able to put its answer down and exit rather than block forever on a send
// nobody is listening for.
//
// A caller that can cancel what it waits on should do that instead; this is the
// last resort for one that cannot.
func withDeadline[T any](what string, timeout time.Duration, call func() (T, error)) (T, error) {
	type answer struct {
		value T
		err   error
	}
	done := make(chan answer, 1)
	go func() {
		value, err := call()
		done <- answer{value: value, err: err}
	}()

	select {
	case a := <-done:
		return a.value, a.err
	case <-time.After(timeout):
		var none T
		return none, fmt.Errorf("%s did not answer within %s: %w", what, timeout, ErrTimedOut)
	}
}
