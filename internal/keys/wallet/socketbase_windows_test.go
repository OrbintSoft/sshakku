//go:build windows

package wallet

import "os"

// shortSocketBase is the directory this system's tests bind their unix sockets
// under. There is no /tmp here; the temporary directory this account was given
// is where a file goes, and it is short enough for a socket address — see
// shortDir.
func shortSocketBase() string { return os.TempDir() }
