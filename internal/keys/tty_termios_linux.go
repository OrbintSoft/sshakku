//go:build linux

package keys

import "golang.org/x/sys/unix"

// tcGetTermiosReq/tcSetTermiosReq are the ioctl request numbers disableEcho
// passes to golang.org/x/sys/unix's Ioctl{Get,Set}Termios — they differ
// between Linux and the BSD family (see tty_termios_other.go).
const (
	tcGetTermiosReq = unix.TCGETS
	tcSetTermiosReq = unix.TCSETS
)
