package reach

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// errReadWriter is an in-process io.ReadWriter that fails Write with writeErr (when
// set), otherwise serves reads from a fixed buffer. It lets identitiesAnswered be
// exercised directly, without a real socket, for the framing edge cases.
type errReadWriter struct {
	writeErr error
	readBuf  []byte
}

func (e *errReadWriter) Write(p []byte) (int, error) {
	if e.writeErr != nil {
		return 0, e.writeErr
	}
	return len(p), nil
}

func (e *errReadWriter) Read(p []byte) (int, error) {
	if len(e.readBuf) == 0 {
		return 0, io.EOF
	}
	n := copy(p, e.readBuf)
	e.readBuf = e.readBuf[n:]
	return n, nil
}

// TestIdentitiesAnsweredEdges covers the write-failure and malformed-frame branches
// of identitiesAnswered that a healthy fake agent never triggers.
func TestIdentitiesAnsweredEdges(t *testing.T) {
	// Each buffer below carries a message type that would be accepted, so the
	// only thing that can make these false is the check being tested. Left
	// empty, the read simply runs out and every one of them would pass with the
	// check deleted.
	answer := []byte{msgIdentitiesAnswer}

	t.Run("write fails", func(t *testing.T) {
		wellFormed := append([]byte{0, 0, 0, 1}, answer...)
		assert.False(t, identitiesAnswered(&errReadWriter{writeErr: errors.New("broken pipe"), readBuf: wellFormed}),
			"a request that could not be written must not be believed answered")
	})
	t.Run("short header", func(t *testing.T) {
		// Fewer than 4 header bytes: io.ReadFull returns before the frame is read.
		assert.False(t, identitiesAnswered(&errReadWriter{readBuf: []byte{0, 0}}),
			"a truncated length header is not an answer")
	})
	t.Run("zero length frame", func(t *testing.T) {
		assert.False(t, identitiesAnswered(&errReadWriter{readBuf: append([]byte{0, 0, 0, 0}, answer...)}),
			"a framed length below 1 is not an answer, whatever byte follows it")
	})
	t.Run("oversized length frame", func(t *testing.T) {
		// length = maxFrame+1, above the cap.
		hdr := make([]byte, 4)
		binary.BigEndian.PutUint32(hdr, maxFrame+1)
		assert.False(t, identitiesAnswered(&errReadWriter{readBuf: append(hdr, answer...)}),
			"a framed length above the cap is not an answer, whatever byte follows it")
	})
	t.Run("type byte truncated", func(t *testing.T) {
		// A valid length of 1 but no message-type byte follows: the second
		// io.ReadFull hits EOF.
		assert.False(t, identitiesAnswered(&errReadWriter{readBuf: []byte{0, 0, 0, 1}}),
			"a missing message type is not an answer")
	})
}

// TestSocketProberSetDeadlineError covers Reachable's bail-out when the dialed
// connection's deadline cannot be set. The dial itself succeeds against a real
// in-process agent, so only the seamed deadline call fails.
func TestSocketProberSetDeadlineError(t *testing.T) {
	orig := setDeadline
	t.Cleanup(func() { setDeadline = orig })
	setDeadline = func(net.Conn, time.Time) error { return errors.New("cannot set deadline") }

	sock := fakeAgent(t, replyIdentities(1))
	assert.False(t, (SocketProber{Timeout: time.Second}).Reachable(t.Context(), sock),
		"a connection whose deadline cannot be set is unreachable")
}
