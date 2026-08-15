//go:build unix

package reach

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestUIDGatedProberReachable(t *testing.T) {
	real := SocketProber{Timeout: time.Second}
	sock := fakeAgent(t, replyIdentities(0))
	ownerUID := os.Getuid()

	t.Run("owned by UID: defers to the wrapped prober", func(t *testing.T) {
		g := UIDGatedProber{UID: ownerUID, Prober: real}
		assert.True(t, g.Reachable(t.Context(), sock), "a socket owned by UID is reachable")
	})
	t.Run("owned by someone else: unreachable without dialing", func(t *testing.T) {
		g := UIDGatedProber{UID: ownerUID + 123456, Prober: real}
		assert.False(t, g.Reachable(t.Context(), sock), "a socket owned by somebody else is unreachable, even though it answers")
	})
	t.Run("empty path", func(t *testing.T) {
		g := UIDGatedProber{UID: ownerUID, Prober: real}
		assert.False(t, g.Reachable(t.Context(), ""), "an empty path is unreachable")
	})
	t.Run("missing socket", func(t *testing.T) {
		g := UIDGatedProber{UID: ownerUID, Prober: real}
		assert.False(t, g.Reachable(t.Context(), filepath.Join(shortDir(t), "nope.sock")), "a missing socket is unreachable")
	})
}

// UIDGatedProber must satisfy Prober.
var _ Prober = UIDGatedProber{}
