package run

import (
	"testing"

	"go.uber.org/goleak"
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
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
