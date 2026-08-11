//go:build darwin

package keys

import (
	"bytes"
	"os"
	"syscall"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// openPTY allocates a pseudo-terminal and returns its master and slave ends,
// both closed when the test finishes. The master is opened non-blocking so the
// runtime poller backs it and SetReadDeadline works, which is what keeps a test
// that drives it from hanging when the expected output never arrives.
//
// Darwin: /dev/ptmx hands out the master, TIOCPTYGRANT and TIOCPTYUNLK take the
// place of Linux's single unlock ioctl, and TIOCPTYGNAME writes the slave's
// path into a caller-supplied buffer — it needs a raw ioctl because x/sys/unix
// exports no pointer-argument helper. O_NOCTTY keeps the slave from becoming
// this process's controlling terminal — only the child that is spawned with
// Setctty may claim it.
func openPTY(t *testing.T) (master, slave *os.File) {
	t.Helper()

	mfd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_NONBLOCK, 0)
	if err != nil {
		t.Skipf("no usable /dev/ptmx in this environment: %v", err)
	}
	master = os.NewFile(uintptr(mfd), "/dev/ptmx")
	t.Cleanup(func() { _ = master.Close() })

	require.NoError(t, unix.IoctlSetInt(mfd, unix.TIOCPTYGRANT, 0), "grant the pty slave")
	require.NoError(t, unix.IoctlSetInt(mfd, unix.TIOCPTYUNLK, 0), "unlock the pty slave")

	// PATH_MAX on Darwin; TIOCPTYGNAME writes a NUL-terminated path into it.
	var buf [1024]byte
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(mfd),
		uintptr(unix.TIOCPTYGNAME), uintptr(unsafe.Pointer(&buf[0])))
	require.Zerof(t, errno, "read the pty slave name: %v", errno)
	name := string(buf[:bytes.IndexByte(buf[:], 0)])

	slave, err = os.OpenFile(name, os.O_RDWR|unix.O_NOCTTY, 0)
	require.NoErrorf(t, err, "open the pty slave %s", name)
	t.Cleanup(func() { _ = slave.Close() })
	return master, slave
}
