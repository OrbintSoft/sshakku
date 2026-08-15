package agent

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain runs the whole package under goleak so a goroutine outliving the
// suite fails the build. Nothing here starts one directly; what these tests
// drive does — the real ssh-agent processes the lifecycle tests start and
// reap, and the socket probes they are judged by — and a probe that never
// finished waiting would otherwise leave the suite green.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
