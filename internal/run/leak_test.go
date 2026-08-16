package run

import (
	"testing"

	"go.uber.org/goleak"

	"github.com/OrbintSoft/sshakku/internal/testproc"
)

// TestMain runs the whole package under goleak: a goroutine still running once
// the suite finishes fails the build.
//
// It matters here more than in most packages. WithDeadline gives up on a call
// without stopping it, so the goroutine carrying that call is expected to
// outlive the wait — the one shape of leak that is deliberate. What goleak
// holds is the other half of that bargain: the goroutine must still be able to
// put its answer down and exit, and a change that left it blocked on a send
// nobody listens for would hold one for the life of the process while every
// assertion here still passed.
// Serve comes first because this binary is also the program the tests here run:
// what a runner does to a process can only be seen by giving it one.
func TestMain(m *testing.M) {
	testproc.Serve()
	goleak.VerifyTestMain(m)
}
