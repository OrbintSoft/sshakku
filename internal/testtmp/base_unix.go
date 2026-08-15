//go:build unix

package testtmp

// socketBase is the directory this system's tests bind their unix sockets
// under. /tmp is short on every unix, which is what a socket address bounded
// at a hundred-odd bytes needs.
func socketBase() string { return "/tmp" }
