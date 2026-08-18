//go:build darwin

package handoff

import (
	"context"
	"os"
	"time"

	"github.com/OrbintSoft/sshakku/internal/paths"
)

// maxSocketAddr is the longest Unix-socket address this system will bind: the
// address field holds 104 bytes, the last of which terminates the path.
const maxSocketAddr = 103

// Stash and Fetch have no kernel keyring to draw on
// (Linux's mechanism, handoff_linux.go) and no tmpfs-backed /tmp for a temp
// file to be a safe stand-in, so Darwin hands the passphrase off over a
// private Unix socket instead (handoff_socket_unix.go): the payload only ever
// exists in a kernel socket buffer, never on disk.
//
// The socket goes under the per-user temporary directory this system gives
// each login ($TMPDIR, a private directory of its own under /var/folders),
// because the address limit above leaves no room for one under a home
// directory of any length: the path a home yields is the home's own plus some
// forty-odd bytes.
func Stash(passphrase string, ttl time.Duration) (string, error) {
	return socketHandoffStash(passphrase, ttl, func() (string, error) {
		return chooseSocketBase(os.Getenv("TMPDIR"), paths.PrivateDir, os.UserCacheDir)
	}, maxSocketAddr)
}

func Fetch(ctx context.Context, token string) (string, error) {
	return socketHandoffFetch(ctx, token)
}

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
//
// It is this system's question and lives with it: the other two do not choose.
// Linux hands nothing over a socket, and Windows has one answer — the account's
// own cache directory, inside a profile that is already the account's.
func chooseSocketBase(tmpDir string, private func(string) bool, cacheDir func() (string, error)) (string, error) {
	if tmpDir != "" && private(tmpDir) {
		return tmpDir, nil
	}
	return cacheDir()
}
