package keys

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"time"
)

// socketHandoffDir returns (creating it if needed) the private per-user
// directory passphrase-handoff sockets live in.
func socketHandoffDir() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cache, "sshakku", "handoff")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// socketHandoffStash listens on a private, randomly-named Unix socket and
// serves passphrase to the first connection, then always closes and removes
// the socket — whether that connection arrived, or ttl elapsed first (e.g.
// ssh-add never invoked the askpass helper), so a stash is never left
// dangling. The returned path is the handoff token socketHandoffFetch dials.
func socketHandoffStash(passphrase string, ttl time.Duration) (string, error) {
	dir, err := socketHandoffDir()
	if err != nil {
		return "", err
	}
	name, err := randomHandoffToken()
	if err != nil {
		return "", err
	}
	sockPath := filepath.Join(dir, name+".sock")

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		return "", err
	}
	if err := os.Chmod(sockPath, 0o600); err != nil {
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
func socketHandoffFetch(token string) (string, error) {
	conn, err := net.Dial("unix", token)
	if err != nil {
		return "", err
	}
	defer func() { _ = conn.Close() }()
	buf, err := io.ReadAll(conn)
	if err != nil {
		return "", err
	}
	return string(buf), nil
}
