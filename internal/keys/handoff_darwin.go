//go:build darwin

package keys

import (
	"os"
	"time"

	"github.com/OrbintSoft/sshakku/internal/paths"
)

// maxSocketAddr is the longest Unix-socket address this system will bind: the
// address field holds 104 bytes, the last of which terminates the path.
const maxSocketAddr = 103

// stashPassphrase and fetchPassphrase have no kernel keyring to draw on
// (Linux's mechanism, handoff_linux.go) and no tmpfs-backed /tmp for a temp
// file to be a safe stand-in, so Darwin hands the passphrase off over a
// private Unix socket instead (handoff_socket.go): the payload only ever
// exists in a kernel socket buffer, never on disk.
//
// The socket goes under the per-user temporary directory this system gives
// each login ($TMPDIR, a private directory of its own under /var/folders),
// because the address limit above leaves no room for one under a home
// directory of any length: the path a home yields is the home's own plus some
// forty-odd bytes.
func stashPassphrase(passphrase string, ttl time.Duration) (string, error) {
	return socketHandoffStash(passphrase, ttl, func() (string, error) {
		return chooseSocketBase(os.Getenv("TMPDIR"), paths.PrivateDir, os.UserCacheDir)
	}, maxSocketAddr)
}

func fetchPassphrase(token string) (string, error) {
	return socketHandoffFetch(token)
}
