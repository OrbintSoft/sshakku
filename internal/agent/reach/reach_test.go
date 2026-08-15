package reach

import (
	"encoding/binary"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeAgent starts an in-process unix listener that handles each connection with
// reply, and returns its socket path.
//
// The accept loop below asserts nothing: it runs on a goroutine of its own, and
// an assertion there would report a failure from outside the test's own
// goroutine. What it serves is the subject's input, not its verdict.
func fakeAgent(t *testing.T, reply func(net.Conn)) string {
	t.Helper()
	sock := filepath.Join(shortDir(t), "a.sock")
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", sock)
	require.NoError(t, err, "listen")
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			reply(c)
			_ = c.Close()
		}
	}()
	return sock
}

// drainRequest reads one framed request so the prober's write completes.
func drainRequest(c net.Conn) {
	var hdr [4]byte
	if _, err := io.ReadFull(c, hdr[:]); err != nil {
		return
	}
	_, _ = io.CopyN(io.Discard, c, int64(binary.BigEndian.Uint32(hdr[:])))
}

// replyIdentities answers a request with an identities-answer listing nkeys keys
// (the keys themselves are omitted — the prober only inspects the message type).
func replyIdentities(nkeys uint32) func(net.Conn) {
	return func(c net.Conn) {
		drainRequest(c)
		payload := []byte{msgIdentitiesAnswer, 0, 0, 0, 0}
		binary.BigEndian.PutUint32(payload[1:], nkeys)
		frame := make([]byte, 4+len(payload))
		binary.BigEndian.PutUint32(frame, uint32(len(payload)))
		copy(frame[4:], payload)
		_, _ = c.Write(frame)
	}
}

func TestSocketProberReachable(t *testing.T) {
	p := SocketProber{Timeout: time.Second}

	t.Run("healthy with keys", func(t *testing.T) {
		assert.True(t, p.Reachable(t.Context(), fakeAgent(t, replyIdentities(2))), "an agent holding keys is reachable")
	})
	t.Run("healthy but empty", func(t *testing.T) {
		assert.True(t, p.Reachable(t.Context(), fakeAgent(t, replyIdentities(0))), "a live agent with no keys is still reachable")
	})
	t.Run("wrong reply type", func(t *testing.T) {
		sock := fakeAgent(t, func(c net.Conn) {
			drainRequest(c)
			_, _ = c.Write([]byte{0, 0, 0, 1, 99}) // not identities-answer
		})
		assert.False(t, p.Reachable(t.Context(), sock), "an unexpected message type is not an agent")
	})
	t.Run("accept then close", func(t *testing.T) {
		sock := fakeAgent(t, func(net.Conn) {}) // reply nothing; conn is closed
		assert.False(t, p.Reachable(t.Context(), sock), "a peer that sends nothing is not an agent")
	})
	t.Run("empty path", func(t *testing.T) {
		assert.False(t, p.Reachable(t.Context(), ""), "an empty path is not an agent")
	})
	t.Run("missing socket", func(t *testing.T) {
		assert.False(t, p.Reachable(t.Context(), filepath.Join(shortDir(t), "nope.sock")), "a missing socket is not an agent")
	})
	t.Run("not a socket", func(t *testing.T) {
		f := filepath.Join(shortDir(t), "regular")
		require.NoError(t, os.WriteFile(f, []byte("x"), 0o600))
		assert.False(t, p.Reachable(t.Context(), f), "a regular file is not an agent")
	})
}

// SocketProber must satisfy Prober.
var _ Prober = SocketProber{}

// TestSocketProberDefaultsItsTimeout pins what an unset Timeout means. The
// deadline the probe puts on the connection is where that choice becomes
// observable, so this reads it there rather than trusting the field it came
// from — a probe that waited forever, or not at all, would answer the same on
// an agent that replies at once.
func TestSocketProberDefaultsItsTimeout(t *testing.T) {
	orig := setDeadline
	t.Cleanup(func() { setDeadline = orig })
	var deadline time.Time
	setDeadline = func(conn net.Conn, dl time.Time) error {
		deadline = dl
		return orig(conn, dl)
	}
	sock := fakeAgent(t, replyIdentities(0))

	t.Run("unset waits the default", func(t *testing.T) {
		before := time.Now()
		require.True(t, (SocketProber{}).Reachable(t.Context(), sock), "the fake agent answers")
		assert.WithinDuration(t, before.Add(DefaultProbeTimeout), deadline, 500*time.Millisecond,
			"a probe given no timeout waits DefaultProbeTimeout — neither forever nor not at all")
	})
	t.Run("a timeout the caller set is the one used", func(t *testing.T) {
		const chosen = 30 * time.Second
		before := time.Now()
		require.True(t, (SocketProber{Timeout: chosen}).Reachable(t.Context(), sock), "the fake agent answers")
		assert.WithinDuration(t, before.Add(chosen), deadline, 500*time.Millisecond,
			"a timeout the caller set must not be replaced by the default")
	})
}
