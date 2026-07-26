package keys

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain runs the whole package under goleak: a goroutine still running once
// the suite finishes fails the build. The passphrase-handoff stash is the only
// production code here that spawns goroutines, and it is one-shot and
// self-cleaning (it exits on the first fetch or when its ttl elapses); goleak
// guards that invariant against regressions.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
