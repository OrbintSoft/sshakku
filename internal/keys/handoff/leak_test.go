package handoff

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain runs the whole package under goleak: a goroutine still running once
// the suite finishes fails the build.
//
// The Darwin stash is what it guards. Putting a passphrase aside there leaves a
// server waiting on Accept for the one helper that will come for it, and that
// server has to end on its own — on the first fetch, or when the ttl elapses
// with nobody having come. One that does not is not a goroutine merely wasted:
// it is holding a passphrase, and it holds it for as long as the process lives.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
