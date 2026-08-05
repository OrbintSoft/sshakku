package keys

import (
	"bytes"
	"os"
	"runtime/pprof"
	"testing"
	"time"
)

// waitGone polls until path is removed or the deadline passes, so the test
// observes the handoff server's own cleanup rather than racing it.
func waitGone(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("socket %s was never cleaned up", path)
}

// TestSocketHandoffNoGoroutineLeak exercises the passphrase-handoff server on
// both paths that must not strand its accept goroutine — a claimed stash and an
// unclaimed one that only ttl closes — and then asks Go's goroutine leak
// detector whether any goroutine is blocked forever. It complements the
// process-wide go.uber.org/goleak check in TestMain, which counts goroutines
// still running at the end of the suite: this one uses the runtime's own
// reachability analysis to catch a goroutine that is permanently blocked.
//
// The detector is only registered when the binary is built with
// GOEXPERIMENT=goroutineleakprofile (see `make test-leakprofile`); without it
// the profile lookup is nil and the test skips.
func TestSocketHandoffNoGoroutineLeak(t *testing.T) {
	prof := pprof.Lookup("goroutineleak")
	if prof == nil {
		t.Skip("goroutineleak profile unavailable; rebuild with GOEXPERIMENT=goroutineleakprofile (make test-leakprofile)")
	}

	base := shortDir(t)

	// Unclaimed stash: the server must time out and exit, never blocking on
	// Accept for a connection that never comes.
	unclaimed, err := socketHandoffStash("s3cr3t", 100*time.Millisecond, base, addrLimit)
	if err != nil {
		t.Fatalf("socketHandoffStash (unclaimed): %v", err)
	}
	waitGone(t, unclaimed)

	// Claimed stash: the server must exit right after serving the one fetch.
	claimed, err := socketHandoffStash("s3cr3t", 5*time.Second, base, addrLimit)
	if err != nil {
		t.Fatalf("socketHandoffStash (claimed): %v", err)
	}
	if _, err := socketHandoffFetch(claimed); err != nil {
		t.Fatalf("socketHandoffFetch: %v", err)
	}
	waitGone(t, claimed)

	// WriteTo runs the leak-detection GC and emits the stacks of any goroutine
	// found leaked; Count then reports how many there are.
	var buf bytes.Buffer
	if err := prof.WriteTo(&buf, 1); err != nil {
		t.Fatalf("write goroutineleak profile: %v", err)
	}
	if n := prof.Count(); n != 0 {
		t.Fatalf("goroutine leak detector found %d leaked goroutine(s):\n%s", n, buf.String())
	}
}
