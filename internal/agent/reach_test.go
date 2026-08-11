package agent

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
	ln, err := net.Listen("unix", sock)
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
		assert.True(t, p.Reachable(fakeAgent(t, replyIdentities(2))), "an agent holding keys is reachable")
	})
	t.Run("healthy but empty", func(t *testing.T) {
		assert.True(t, p.Reachable(fakeAgent(t, replyIdentities(0))), "a live agent with no keys is still reachable")
	})
	t.Run("wrong reply type", func(t *testing.T) {
		sock := fakeAgent(t, func(c net.Conn) {
			drainRequest(c)
			_, _ = c.Write([]byte{0, 0, 0, 1, 99}) // not identities-answer
		})
		assert.False(t, p.Reachable(sock), "an unexpected message type is not an agent")
	})
	t.Run("accept then close", func(t *testing.T) {
		sock := fakeAgent(t, func(net.Conn) {}) // reply nothing; conn is closed
		assert.False(t, p.Reachable(sock), "a peer that sends nothing is not an agent")
	})
	t.Run("empty path", func(t *testing.T) {
		assert.False(t, p.Reachable(""), "an empty path is not an agent")
	})
	t.Run("missing socket", func(t *testing.T) {
		assert.False(t, p.Reachable(filepath.Join(shortDir(t), "nope.sock")), "a missing socket is not an agent")
	})
	t.Run("not a socket", func(t *testing.T) {
		f := filepath.Join(shortDir(t), "regular")
		require.NoError(t, os.WriteFile(f, []byte("x"), 0o600))
		assert.False(t, p.Reachable(f), "a regular file is not an agent")
	})
}

// SocketProber must satisfy Prober.
var _ Prober = SocketProber{}
