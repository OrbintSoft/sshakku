//go:build darwin

package keys

import "time"

// stashPassphrase and fetchPassphrase have no kernel keyring to draw on
// (Linux's mechanism, handoff_linux.go) and no tmpfs-backed /tmp for a temp
// file to be a safe stand-in, so Darwin hands the passphrase off over a
// private Unix socket instead (handoff_socket.go): the payload only ever
// exists in a kernel socket buffer, never on disk.
func stashPassphrase(passphrase string, ttl time.Duration) (string, error) {
	return socketHandoffStash(passphrase, ttl)
}

func fetchPassphrase(token string) (string, error) {
	return socketHandoffFetch(token)
}
