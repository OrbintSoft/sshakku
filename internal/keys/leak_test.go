package keys

import (
	"testing"

	"go.uber.org/goleak"

	"github.com/OrbintSoft/sshakku/internal/testproc"
)

// TestMain runs the whole package under goleak: a goroutine still running once
// the suite finishes fails the build.
//
// Nothing here starts one today, which is the state worth holding: everything
// this package does on the user's behalf happens while a login shell waits for
// it, so work outsourced to a goroutine that outlives the call is work whose
// failure nobody is left to notice.
// Serve comes first because this binary is also the program one test here runs:
// what a runner hands a process on its standard input can only be seen from a
// process that was really handed it.
func TestMain(m *testing.M) {
	testproc.Serve()
	goleak.VerifyTestMain(m)
}
