//go:build unix

package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUIDGatedProberReachable(t *testing.T) {
	real := SocketProber{Timeout: time.Second}
	sock := fakeAgent(t, replyIdentities(0))
	ownerUID := os.Getuid()

	t.Run("owned by UID: defers to the wrapped prober", func(t *testing.T) {
		g := UIDGatedProber{UID: ownerUID, Prober: real}
		if !g.Reachable(sock) {
			t.Fatal("want reachable: socket is owned by UID")
		}
	})
	t.Run("owned by someone else: unreachable without dialing", func(t *testing.T) {
		g := UIDGatedProber{UID: ownerUID + 123456, Prober: real}
		if g.Reachable(sock) {
			t.Fatal("want unreachable: socket is not owned by UID, even though it answers")
		}
	})
	t.Run("empty path", func(t *testing.T) {
		g := UIDGatedProber{UID: ownerUID, Prober: real}
		if g.Reachable("") {
			t.Fatal("want unreachable for an empty path")
		}
	})
	t.Run("missing socket", func(t *testing.T) {
		g := UIDGatedProber{UID: ownerUID, Prober: real}
		if g.Reachable(filepath.Join(shortDir(t), "nope.sock")) {
			t.Fatal("want unreachable for a missing socket")
		}
	})
}

// UIDGatedProber must satisfy Prober.
var _ Prober = UIDGatedProber{}
