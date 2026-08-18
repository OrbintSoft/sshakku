//go:build windows

package handoff

import (
	"context"
	"os"
	"time"
)

// maxSocketAddr is the longest socket address this system will bind: the
// address field holds 108 bytes, the last of which terminates the path.
const maxSocketAddr = 107

// Stash and Fetch hand the passphrase over a private socket, the same
// rendezvous Darwin uses (handoff_socket.go): the payload only ever exists in
// a kernel socket buffer, and what crosses the environment is a token that is
// worth nothing on its own.
//
// The socket goes under this account's own cache directory, which is inside
// the profile and is therefore the account's by construction — there is no
// shared directory to be waited in, and none to be chosen wrongly. That is
// also where a system with no permission bits on a socket puts privacy: in the
// directory rather than on the file (handoff_privacy_windows.go).
func Stash(passphrase string, ttl time.Duration) (string, error) {
	return socketHandoffStash(passphrase, ttl, os.UserCacheDir, maxSocketAddr)
}

// Fetch reads the one passphrase the token's socket serves.
func Fetch(ctx context.Context, token string) (string, error) {
	return socketHandoffFetch(ctx, token)
}
