// Package logline holds what every caller of the session log had been spelling
// out for itself: format a line, record it, and carry on whatever happens.
//
// It exists for the two decisions in that sentence rather than for the typing it
// saves. A nil logger records nothing instead of panicking, because a logger is
// something a caller may not have been given and every one of them would
// otherwise have to remember that. And the error is dropped, because a log that
// cannot be written is not a reason to fail the thing being logged about — this
// program's callers are on the path of somebody's login shell, and a session
// that will not open because its log file is full would be the worse outcome by
// far. Both were already the rule everywhere; only now there is one place to
// read it, and one place to change it.
package logline

import "fmt"

// Logger records one level-tagged line.
//
// Packages that log declare an interface of this shape for themselves, naming
// what they want it for; those are assignable to this one, so nothing has to
// import this package to be logged through it.
type Logger interface {
	Log(level, message string) error
}

// Recordf formats one line and records it at the given level. A nil Logger
// records nothing, and a Logger that fails is not reported: see the package
// comment for why both are deliberate.
func Recordf(log Logger, level, format string, args ...any) {
	if log == nil {
		return
	}
	_ = log.Log(level, fmt.Sprintf(format, args...))
}
