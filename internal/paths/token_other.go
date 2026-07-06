//go:build !linux

package paths

// SocketToken returns no token on platforms without a Linux kernel keyring, so
// the caller degrades to a tokenless socket path.
func SocketToken() string { return "" }

// ReadSocketToken returns no token on platforms without a Linux kernel keyring.
func ReadSocketToken() string { return "" }
