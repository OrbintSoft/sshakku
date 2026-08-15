package keys

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain runs the whole package under goleak: a goroutine still running once
// the suite finishes fails the build.
//
// Nothing here starts one today, which is the state worth holding: everything
// this package does on the user's behalf happens while a login shell waits for
// it, so work outsourced to a goroutine that outlives the call is work whose
// failure nobody is left to notice.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
