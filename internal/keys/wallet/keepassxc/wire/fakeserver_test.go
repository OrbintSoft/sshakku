package wire

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fakeServer is a stand-in for KeePassXC that speaks the real protocol: it
// generates its own key pair, decrypts what the client sends with NaCl box, and
// encrypts its replies back under the incremented nonce.
//
// It deliberately does not fake the crypto or the framing. Those are what the
// client under test exists to get right, and a stand-in that skipped them would
// let a client that encrypts to the wrong key, or checks no nonce, pass.
//
// The server runs on its own goroutine, so everything it records is behind a
// mutex and read through the accessors below.
type fakeServer struct {
	t    *testing.T
	ln   net.Listener
	path string
	keys keyPair

	// reply returns the inner payload for an action. A nil result is answered
	// with a bare success.
	reply func(action string, inner map[string]any) any

	// failWith, when set for an action, is sent instead of an encrypted
	// payload — how KeePassXC reports a locked database or a refused
	// association.
	failWith map[string]envelope

	// prelude is written before the reply to the named action: KeePassXC
	// pushes unsolicited frames whenever the database locks or unlocks, and
	// they land interleaved with real replies.
	prelude map[string][]envelope

	// sealUnderWrongNonce encrypts the reply under a nonce the client is not
	// expecting, standing in for a reply that belongs to some other request.
	sealUnderWrongNonce bool

	// hangUpOn names an action after which the server drops the connection
	// without answering, standing in for KeePassXC exiting mid-session.
	hangUpOn string

	// replyAfter delays the answer to an action, standing in for an exchange
	// that waits on something slower than a socket — a dialog somebody has to
	// read, for instance.
	replyAfter map[string]time.Duration

	mu sync.Mutex
	// clientPub is learned from the key exchange and is what replies are
	// encrypted to.
	clientPub *[keyLen]byte
	// raw accumulates every byte the client sent, so a test can assert on what
	// actually crossed the socket rather than on what was decrypted.
	raw bytes.Buffer
	// decrypted records each inner payload the server opened.
	decrypted []map[string]any
}

// newFakeServer starts a server on a unix socket and stops it when the test
// ends.
func newFakeServer(t *testing.T) *fakeServer {
	t.Helper()
	keys, err := newKeyPair()
	require.NoError(t, err, "generating the server key pair")
	path := filepath.Join(shortSocketDir(t), "s")
	ln, err := (&net.ListenConfig{}).Listen(t.Context(), "unix", path)
	require.NoErrorf(t, err, "listening on %s", path)
	s := &fakeServer{
		t:        t,
		ln:       ln,
		path:     path,
		keys:     keys,
		failWith: map[string]envelope{},
		prelude:  map[string][]envelope{},
	}
	t.Cleanup(func() { _ = ln.Close() })
	go s.serve()
	return s
}

// shortSocketDir returns a directory a unix socket can actually live in.
//
// A socket *address* is capped at 104 bytes on Darwin (108 on Linux and on
// Windows), and the name is checked at bind, not at connect. t.TempDir() is
// nowhere near short enough on macOS, where TMPDIR is a per-user path under
// /var/folders and the test's own name is appended to it — so the bind fails
// with EINVAL and every test in the file goes red at once. Which directory is
// short enough to hold one is the platform's answer, not this function's:
// shortSocketBase gives it.
func shortSocketDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp(shortSocketBase(), "kpxc") //nolint:usetesting // t.TempDir() is the long macOS path the comment above is about
	require.NoError(t, err, "mkdir temp")
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// rawBytes returns everything the client has sent so far.
func (s *fakeServer) rawBytes() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return bytes.Clone(s.raw.Bytes())
}

// openedPayloads returns the inner payloads the server decrypted.
func (s *fakeServer) openedPayloads() []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]map[string]any(nil), s.decrypted...)
}

// dial connects a client to the server and closes it when the test ends.
func (s *fakeServer) dial() net.Conn {
	s.t.Helper()
	conn, err := (&net.Dialer{}).DialContext(s.t.Context(), "unix", s.path)
	require.NoErrorf(s.t, err, "dialling %s", s.path)
	s.t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// connect builds a client against the server, failing the test if the exchange
// does not complete.
func (s *fakeServer) connect() *Client {
	s.t.Helper()
	c, err := Connect(s.dial(), 5*time.Second, 5*time.Second)
	require.NoError(s.t, err, "Connect")
	return c
}

// serve accepts one connection and answers frames until it closes. Everything
// read is teed into s.raw so a test can inspect the wire.
func (s *fakeServer) serve() {
	conn, err := s.ln.Accept()
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()

	dec := json.NewDecoder(io.TeeReader(conn, &lockedWriter{s: s}))
	for {
		var req envelope
		if err := dec.Decode(&req); err != nil {
			return
		}
		if err := s.handle(conn, req); err != nil {
			return
		}
	}
}

// lockedWriter guards the recording buffer, which the test goroutine reads.
type lockedWriter struct{ s *fakeServer }

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.s.mu.Lock()
	defer w.s.mu.Unlock()
	return w.s.raw.Write(p)
}

// handle answers one frame.
func (s *fakeServer) handle(conn net.Conn, req envelope) error {
	for _, frame := range s.prelude[req.Action] {
		if err := writeJSON(conn, frame); err != nil {
			return err
		}
	}

	if s.hangUpOn != "" && req.Action == s.hangUpOn {
		return errServerHungUp
	}

	if delay := s.replyAfter[req.Action]; delay > 0 {
		time.Sleep(delay)
	}

	// The key exchange is answered before the failures below only when it is
	// not one of them: it is the one frame a real KeePassXC can also refuse in
	// the clear, and a fake that always accepted it would make that refusal
	// impossible to test.
	if _, refused := s.failWith[actionChangePublicKeys]; req.Action == actionChangePublicKeys && !refused {
		return s.handleKeyExchange(conn, req)
	}

	if failure, ok := s.failWith[req.Action]; ok {
		failure.Action = req.Action
		return writeJSON(conn, failure)
	}

	nonce, err := decodeNonce(req.Nonce)
	if err != nil {
		return err
	}
	clientPub, err := s.clientKey()
	if err != nil {
		return err
	}
	var inner map[string]any
	if err := open(req.Message, nonce, clientPub, s.keys.secret, &inner); err != nil {
		return err
	}
	s.record(inner)

	var payload any
	if s.reply != nil {
		payload = s.reply(req.Action, inner)
	}
	if payload == nil {
		payload = map[string]any{"success": "true"}
	}

	replyNonce := incrementNonce(nonce)
	if s.sealUnderWrongNonce {
		replyNonce = incrementNonce(replyNonce)
	}
	sealed, err := seal(payload, replyNonce, clientPub, s.keys.secret)
	if err != nil {
		return err
	}
	// No success on the envelope: a real KeePassXC states acceptance only
	// inside the encrypted message, and names a failure outside. A fake that
	// also set it here would let a client that reads the wrong one pass.
	return writeJSON(conn, envelope{
		Action:  req.Action,
		Message: sealed,
		Nonce:   base64.StdEncoding.EncodeToString(replyNonce[:]),
	})
}

// handleKeyExchange answers the one frame that travels in the clear.
func (s *fakeServer) handleKeyExchange(conn net.Conn, req envelope) error {
	clientPub, err := decodeKey(req.PublicKey)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.clientPub = clientPub
	s.mu.Unlock()
	return writeJSON(conn, envelope{
		Action:    actionChangePublicKeys,
		PublicKey: base64.StdEncoding.EncodeToString(s.keys.public[:]),
		Nonce:     req.Nonce,
		Success:   "true",
	})
}

// clientKey returns the key learned from the exchange.
func (s *fakeServer) clientKey() (*[keyLen]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.clientPub == nil {
		return nil, fmt.Errorf("the client sent an encrypted frame before exchanging keys")
	}
	return s.clientPub, nil
}

// record stores a decrypted payload.
func (s *fakeServer) record(inner map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.decrypted = append(s.decrypted, inner)
}

// errServerHungUp ends serve, which closes the connection.
var errServerHungUp = fmt.Errorf("the server hung up")

// writeJSON sends one frame.
func writeJSON(conn net.Conn, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	_, err = conn.Write(raw)
	return err
}

// decodeNonce parses a base64 nonce of exactly nonceLen bytes.
func decodeNonce(encoded string) ([nonceLen]byte, error) {
	var nonce [nonceLen]byte
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nonce, err
	}
	if len(raw) != nonceLen {
		return nonce, fmt.Errorf("a nonce must be %d bytes, got %d", nonceLen, len(raw))
	}
	copy(nonce[:], raw)
	return nonce, nil
}
