package agent

import (
	"testing"

	"go.uber.org/goleak"

	"github.com/OrbintSoft/sshakku/internal/testproc"
)

// TestMain runs the whole package under goleak so a goroutine outliving the
// suite fails the build. Nothing here starts one directly; what these tests
// drive does — the real ssh-agent processes the lifecycle tests start and
// reap, and the socket probes they are judged by — and a probe that never
// finished waiting would otherwise leave the suite green.
// Serve comes first because this binary stands in for an ssh-agent that
// refuses: two tests need a process that really exits non-zero, which no stub
// for the exec boundary can be.
func TestMain(m *testing.M) {
	testproc.Serve()
	goleak.VerifyTestMain(m)
}
