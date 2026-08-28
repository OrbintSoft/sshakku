// Package reach answers one question about an ssh-agent: does it answer.
//
// It asks the way `ssh-add -l` does — by speaking the agent's own wire
// protocol on its socket — because a socket file being present says nothing
// about whether anything is still behind it, which is exactly the state a
// crashed agent leaves behind.
package reach

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"time"
)

// ssh-agent wire-protocol message types we use (OpenSSH PROTOCOL.agent).
const (
	msgRequestIdentities = 11 // SSH_AGENTC_REQUEST_IDENTITIES.
	msgIdentitiesAnswer  = 12 // SSH_AGENT_IDENTITIES_ANSWER.
)

// DefaultProbeTimeout bounds a reachability probe, matching the login script's
// `timeout 2 ssh-add -l`.
const DefaultProbeTimeout = 2 * time.Second

// Prober reports whether a usable ssh-agent answers on a unix socket path.
// UIDGatedProber wraps one, which is why the interface is stated here as well
// as where the agent lifecycle consumes it.
type Prober interface {
	Reachable(ctx context.Context, socket string) bool
}

// maxFrame caps the response length we will read, so a malformed or hostile peer
// cannot make us allocate unbounded memory. We only need the first payload byte.
const maxFrame = 256 << 10

// setDeadline applies a read/write deadline to conn. It is a package variable so
// the failure path can be tested without a connection whose deadline genuinely
// cannot be set.
var setDeadline = func(conn net.Conn, t time.Time) error { return conn.SetDeadline(t) }

// SocketProber probes a real ssh-agent by dialing its unix socket and issuing a
// minimal request-identities ping. A valid identities-answer — regardless of how
// many keys it lists — means the agent is healthy.
type SocketProber struct {
	// Timeout bounds dial + request + response; zero means DefaultProbeTimeout.
	Timeout time.Duration
}

// Reachable reports whether an ssh-agent answers on socket.
func (p SocketProber) Reachable(ctx context.Context, socket string) bool {
	if socket == "" {
		return false
	}
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = DefaultProbeTimeout
	}
	conn, err := (&net.Dialer{Timeout: timeout}).DialContext(ctx, "unix", socket)
	if err != nil {
		return false
	}
	defer func() { _ = conn.Close() }()
	if err := setDeadline(conn, time.Now().Add(timeout)); err != nil {
		return false
	}
	return identitiesAnswered(conn)
}

// identitiesAnswered sends SSH_AGENTC_REQUEST_IDENTITIES and reports whether the
// peer replies with an SSH_AGENT_IDENTITIES_ANSWER message.
func identitiesAnswered(conn io.ReadWriter) bool {
	req := [...]byte{0, 0, 0, 1, msgRequestIdentities}
	if _, err := conn.Write(req[:]); err != nil {
		return false
	}
	var header [4]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		return false
	}
	length := binary.BigEndian.Uint32(header[:])
	if length < 1 || length > maxFrame {
		return false
	}
	var msgType [1]byte
	if _, err := io.ReadFull(conn, msgType[:]); err != nil {
		return false
	}
	return msgType[0] == msgIdentitiesAnswer
}
