package logline

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recorder is a Logger that keeps what it was told, and fails on demand so the
// caller's indifference to that can be checked rather than assumed.
type recorder struct {
	levels   []string
	messages []string
	err      error
}

func (r *recorder) Log(level, message string) error {
	r.levels = append(r.levels, level)
	r.messages = append(r.messages, message)
	return r.err
}

func TestRecordfFormatsTheLineAndItsLevel(t *testing.T) {
	rec := &recorder{}
	Recordf(rec, "ERROR", "%s could not ask for %s (%v)", "zenity", "id_rsa", errors.New("no display"))

	require.Len(t, rec.messages, 1, "one call records one line")
	assert.Equal(t, "ERROR", rec.levels[0], "the level is passed through, not folded into the message")
	assert.Equal(t, "zenity could not ask for id_rsa (no display)", rec.messages[0], "the recorded line")
}

// TestRecordfWithNoLoggerRecordsNothing pins the first of the two decisions this
// package exists for. A caller may not have been given a logger, and every one
// of them would otherwise have to remember that before every call.
func TestRecordfWithNoLoggerRecordsNothing(t *testing.T) {
	assert.NotPanics(t, func() { Recordf(nil, "INFO", "nobody is listening") },
		"a nil Logger records nothing rather than bringing down the caller")
}

// TestRecordfIgnoresALoggerThatFails pins the second. A log that cannot be
// written is not a reason to fail the thing being logged about: these callers
// are on the path of somebody's login shell.
func TestRecordfIgnoresALoggerThatFails(t *testing.T) {
	rec := &recorder{err: errors.New("no space left on device")}
	assert.NotPanics(t, func() { Recordf(rec, "INFO", "loaded %s", "id_rsa") },
		"a Logger that fails is not reported and does not stop the caller")
	assert.Len(t, rec.messages, 1, "the line was still offered to the Logger")
}

// TestRecordfTakesAnyLoggerOfTheRightShape is why the packages that log need not
// import this one: they declare an interface of their own, naming what they want
// it for, and it is assignable here.
func TestRecordfTakesAnyLoggerOfTheRightShape(t *testing.T) {
	type callerOwnLogger interface {
		Log(level, message string) error
	}

	rec := &recorder{}
	var theirs callerOwnLogger = rec
	Recordf(theirs, "WARN", "adopted an agent this session did not start")

	require.Len(t, rec.messages, 1, "a Logger declared elsewhere records through this one")
	assert.Equal(t, "WARN", rec.levels[0], "the level of a line recorded through a caller's own interface")
}
