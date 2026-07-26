package agent

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain runs the whole package under goleak so a goroutine outliving the
// suite fails the build. The package's own goroutines are the in-process fake
// agents the reachability tests spin up; each is torn down with its listener in
// a t.Cleanup, and goleak verifies none is left running.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
