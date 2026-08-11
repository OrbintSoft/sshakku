//go:build windows

package agent

import (
	"errors"
	"time"
)

// errNoLock is what FlockLocker reports here. Windows locks a byte range of an
// open file rather than the file itself, which is a different mechanism from
// the advisory whole-file lock the Unix build takes, and it is not written yet.
var errNoLock = errors.New("locking the agent start path is not implemented on windows")

// FlockLocker serialises the mutate path of EnsureAgent. Its fields carry the
// caller's waiting policy, which nothing here reads yet.
type FlockLocker struct {
	// Wait bounds how long to block for the lock.
	Wait time.Duration
	// Poll is the retry interval while waiting.
	Poll time.Duration
}

// Lock reports errNoLock. It refuses rather than returning a release function
// that locks nothing: the caller's own fallback is to proceed unserialised
// knowingly, and that is a decision to leave with the caller.
func (FlockLocker) Lock(string) (func(), error) { return nil, errNoLock }

var _ Locker = FlockLocker{}
