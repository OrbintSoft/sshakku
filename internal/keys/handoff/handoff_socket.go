// The passphrase handoff over a private socket. Darwin reaches for it
// (handoff_darwin.go) and so does Windows (handoff_windows.go); Linux has the
// kernel keyring instead. The rendezvous itself is the same everywhere — a
// socket nobody else can guess the name of, serving one connection and then
// gone — and only what makes it private differs, which is why that step is
// each system's own (handoff_privacy_*.go) and everything here is not.

package handoff

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/OrbintSoft/sshakku/internal/run"
)

// Seams over the socket listen, read and deadline, so the failure branches on
// either side of the rendezvous are exercisable deterministically. Production
// points them at the real net and io operations; what makes the socket private
// is a seam too, and lives with the system that decides it.
var (
	netListen   = net.Listen
	readAll     = io.ReadAll
	setDeadline = func(conn net.Conn, t time.Time) error { return conn.SetDeadline(t) }
)

// fetchBudget bounds the collecting of a passphrase from the rendezvous.
//
// It is the budget of something expected to answer by itself rather than of a
// wait on a person: whoever had to type the passphrase has already typed it,
// and what is being waited on here is a server this program started, on this
// machine, holding an answer it writes the moment a caller arrives. A read that
// has not finished in this long is not slow, it is one that is not going to
// finish.
//
// A var rather than a const so a test can shorten it, since what it bounds is
// the one thing a test of it must not actually spend.
var fetchBudget = run.DefaultCommandTimeout

// socketHandoffDir returns (creating it if needed) the private per-user
// directory passphrase-handoff sockets live in, under base. Named "h", not
// "handoff": every byte here counts against the socket address limit — barely
// a hundred bytes, wherever this runs — once the socket filename is appended.
//
// The leaf is forced to 0700 rather than left to the umask: it holds a
// rendezvous for a passphrase, and one that already existed with looser
// permissions must not be inherited as it stands.
func socketHandoffDir(base string) (string, error) {
	dir := filepath.Join(base, "sshakku", "h")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	if err := chmodDir(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// socketPathTooLongError is a socket address this system will not accept. The
// kernel refuses one with "invalid argument", which says nothing about length
// and sends whoever reads it looking for a permission or a missing directory,
// so the length is spelled out here instead.
type socketPathTooLongError struct {
	path    string
	allowed int
}

func (e socketPathTooLongError) Error() string {
	return fmt.Sprintf("the passphrase needs a socket at %s, which is %d bytes where this system allows at most %d for a socket address",
		e.path, len(e.path), e.allowed)
}

// socketHandoffStash listens on a private, randomly-named Unix socket and
// serves passphrase to the first connection, then always closes and removes
// the socket — whether that connection arrived, or ttl elapsed first (e.g.
// ssh-add never invoked the askpass helper), so a stash is never left
// dangling. The returned path is the handoff token socketHandoffFetch dials.
func socketHandoffStash(passphrase string, ttl time.Duration, base func() (string, error), maxAddr int) (string, error) {
	root, err := base()
	if err != nil {
		return "", err
	}
	dir, err := socketHandoffDir(root)
	if err != nil {
		return "", err
	}
	name, err := randomToken()
	if err != nil {
		return "", err
	}
	sockPath := filepath.Join(dir, name+".sock")
	// The kernel refuses an address this long with "invalid argument", which
	// says nothing about length and leaves whoever reads it looking for a
	// permission or a missing directory instead.
	if len(sockPath) > maxAddr {
		return "", socketPathTooLongError{path: sockPath, allowed: maxAddr}
	}

	ln, err := netListen("unix", sockPath)
	if err != nil {
		return "", err
	}
	if err := chmodSock(sockPath, 0o600); err != nil {
		_ = ln.Close()
		_ = os.Remove(sockPath)
		return "", err
	}

	go serveSocketHandoffOnce(ln, sockPath, passphrase, ttl)
	return sockPath, nil
}

// serveSocketHandoffOnce accepts at most one connection and writes passphrase
// to it, giving up once ttl elapses, and always cleans up the listener and
// socket file afterward — the one-shot, self-cleaning mirror of
// handoff_linux.go's keyring-read-then-unlink.
func serveSocketHandoffOnce(ln net.Listener, sockPath, passphrase string, ttl time.Duration) {
	defer func() {
		_ = ln.Close()
		_ = os.Remove(sockPath)
	}()

	type result struct {
		conn net.Conn
		err  error
	}
	accepted := make(chan result, 1)
	go func() {
		conn, err := ln.Accept()
		accepted <- result{conn, err}
	}()

	select {
	case r := <-accepted:
		if r.err == nil {
			_, _ = r.conn.Write([]byte(passphrase))
			_ = r.conn.Close()
		}
	case <-time.After(ttl):
	}
}

// errNothingHandedOver is what a collector is told when the rendezvous was
// there to dial but nothing came across it. A stash is dialable for as long as
// it takes to close the listener and unlink the socket after serving its one
// connection, so a second collector inside that gap is accepted and served no
// byte — and an empty passphrase is what an empty read otherwise looks like.
// Only one of the two is an answer: a passphrase nobody typed spends the
// attempt ssh would have given the person who could have typed it.
var errNothingHandedOver = errors.New("the passphrase was not handed over: the handoff had already been collected")

// socketHandoffFetch dials the socket token names and reads the one
// passphrase it serves, reporting a rendezvous that served none as the failed
// handoff it is rather than as a passphrase.
func socketHandoffFetch(ctx context.Context, token string) (string, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", token)
	if err != nil {
		return "", err
	}
	defer func() { _ = conn.Close() }()
	if err = setDeadline(conn, collectBy(ctx)); err != nil {
		return "", err
	}
	buf, err := readAll(conn)
	if err != nil {
		return "", err
	}
	if len(buf) == 0 {
		return "", errNothingHandedOver
	}
	return string(buf), nil
}

// collectBy is the moment collecting gives up: the caller's own deadline where
// they set one that falls sooner, and this package's budget otherwise.
//
// The dial honours the context and the read would not, which is the whole of
// what this exists to put right — a caller that has already stopped waiting is
// left holding a read that has not.
func collectBy(ctx context.Context) time.Time {
	by := time.Now().Add(fetchBudget)
	if theirs, ok := ctx.Deadline(); ok && theirs.Before(by) {
		return theirs
	}
	return by
}
