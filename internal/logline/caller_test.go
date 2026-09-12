// The test here is written from outside the package deliberately. What it
// checks is that a package which logs need not import this one, and a test
// living inside logline could only act that out — its interface would be one
// more declaration in the package that already declares Logger, which is the
// arrangement being ruled out rather than the one being checked.
package logline_test

import (
	"testing"

	"github.com/OrbintSoft/sshakku/internal/logline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// callerOwnLogger is what a package that logs declares for itself: the one
// method it wants a logger for, named where it is used rather than imported.
type callerOwnLogger interface {
	Log(level, message string) error
}

// callerOwnRecorder is such a package's own Logger, keeping what it was told so
// the line can be read back.
type callerOwnRecorder struct {
	levels   []string
	messages []string
}

func (r *callerOwnRecorder) Log(level, message string) error {
	r.levels = append(r.levels, level)
	r.messages = append(r.messages, message)
	return nil
}

// TestRecordfTakesAnyLoggerOfTheRightShape is why the packages that log need not
// import this one: they declare an interface of their own, naming what they want
// it for, and it is assignable here.
func TestRecordfTakesAnyLoggerOfTheRightShape(t *testing.T) {
	rec := &callerOwnRecorder{}
	var theirs callerOwnLogger = rec
	logline.Recordf(theirs, "WARN", "adopted an agent this session did not start")

	require.Len(t, rec.messages, 1, "a Logger declared elsewhere records through this one")
	assert.Equal(t, "WARN", rec.levels[0], "the level of a line recorded through a caller's own interface")
}
