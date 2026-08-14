//go:build unix

package wallet

// shortSocketBase is the directory this system's tests bind their unix sockets
// under. /tmp is short on every unix, which is what a socket address bounded
// at a hundred-odd bytes needs — see shortDir.
func shortSocketBase() string { return "/tmp" }
