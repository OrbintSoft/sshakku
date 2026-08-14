// The passphrase handoff over a private Unix socket. Darwin is what reaches
// for it (handoff_darwin.go); Linux has the kernel keyring instead, and
// Windows has neither and says so — see handoff_windows.go. It is built for
// the family whose sockets carry a permission bit, because that bit is what
// makes the rendezvous private: a system that answers "who may connect" with
// an access-control list needs a mechanism of its own, not this one with the
// check left out.

//go:build unix

package keys

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"
)

// Seams over the socket listen and chmod, so socketHandoffStash's listen- and
// permission-failure branches are exercisable deterministically. Production
// points them at the real net and os operations.
var (
	netListen = net.Listen
	chmodDir  = os.Chmod
	chmodSock = os.Chmod
	readAll   = io.ReadAll
)

// chooseSocketBase picks the directory handoff sockets are created under.
//
// A per-user temporary directory the system hands out (tmpDir) is preferred:
// it is short, which matters because a socket address is capped at barely a
// hundred bytes, and it is the same private per-user directory the cache is.
// It is only taken if it really is that, though — private reports whether it
// exists, belongs to this user, and grants nothing to anyone else — because
// unlike the cache directory it is named by the environment rather than
// derived from the user's own home, and a shared directory (a bare /tmp, say)
// is one an attacker can wait in for a passphrase.
//
// Anything else falls back to the cache directory, which is private by
// construction but sits under the home and is therefore as long as the home
// is.
func chooseSocketBase(tmpDir string, private func(string) bool, cacheDir func() (string, error)) (string, error) {
	if tmpDir != "" && private(tmpDir) {
		return tmpDir, nil
	}
	return cacheDir()
}

// socketHandoffDir returns (creating it if needed) the private per-user
// directory passphrase-handoff sockets live in, under base. Named "h", not
// "handoff": every byte here counts against AF_UNIX's sun_path limit (104
// bytes on Darwin, 108 on Linux) once the socket filename is appended.
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
	name, err := randomHandoffToken()
	if err != nil {
		return "", err
	}
	sockPath := filepath.Join(dir, name+".sock")
	// The kernel refuses an address this long with "invalid argument", which
	// says nothing about length and leaves whoever reads it looking for a
	// permission or a missing directory instead.
	if len(sockPath) > maxAddr {
		return "", fmt.Errorf("the passphrase needs a socket at %s, which is %d bytes where this system allows at most %d for a socket address", sockPath, len(sockPath), maxAddr)
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

// socketHandoffFetch dials the socket token names and reads the one
// passphrase it serves.
func socketHandoffFetch(ctx context.Context, token string) (string, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", token)
	if err != nil {
		return "", err
	}
	defer func() { _ = conn.Close() }()
	buf, err := readAll(conn)
	if err != nil {
		return "", err
	}
	return string(buf), nil
}
