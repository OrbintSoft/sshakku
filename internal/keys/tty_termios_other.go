//go:build unix && !linux

package keys

import "golang.org/x/sys/unix"

// tcGetTermiosReq/tcSetTermiosReq: the BSD family (including Darwin) names
// these ioctl requests TIOCGETA/TIOCSETA rather than Linux's TCGETS/TCSETS
// (see tty_termios_linux.go).
const (
	tcGetTermiosReq = unix.TIOCGETA
	tcSetTermiosReq = unix.TIOCSETA
)
