//go:build unix

package handoff

import (
	"bytes"
	"os"
	"runtime/pprof"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/testtmp"
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
	require.FailNowf(t, "a rendezvous was never cleaned up",
		"the socket %s is still there, so the passphrase sits where anything that can reach it could take it", path)
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

	base := testtmp.ShortDir(t)

	// Unclaimed stash: the server must time out and exit, never blocking on
	// Accept for a connection that never comes.
	unclaimed, err := socketHandoffStash("s3cr3t", 100*time.Millisecond, fixedBase(base), addrLimit)
	require.NoError(t, err, "putting a passphrase aside must succeed")
	waitGone(t, unclaimed)

	// Claimed stash: the server must exit right after serving the one fetch.
	claimed, err := socketHandoffStash("s3cr3t", 5*time.Second, fixedBase(base), addrLimit)
	require.NoError(t, err, "putting a second passphrase aside must succeed")
	_, err = socketHandoffFetch(t.Context(), claimed)
	require.NoError(t, err, "and the helper that was meant to have it must get it")
	waitGone(t, claimed)

	// WriteTo runs the leak-detection GC and emits the stacks of any goroutine
	// found leaked; Count then reports how many there are.
	var buf bytes.Buffer
	require.NoError(t, prof.WriteTo(&buf, 1), "run the leak-detection GC")
	assert.Zerof(t, prof.Count(),
		"a handoff server left blocked on Accept holds a passphrase and a goroutine for the life of the process:\n%s",
		buf.String())
}
