//go:build linux

package keys

import (
	"fmt"
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

// openPTY allocates a pseudo-terminal and returns its master and slave ends,
// both closed when the test finishes. The master is opened non-blocking so the
// runtime poller backs it and SetReadDeadline works, which is what keeps a test
// that drives it from hanging when the expected output never arrives.
//
// Linux: /dev/ptmx hands out the master, TIOCSPTLCK unlocks the slave, and
// TIOCGPTN names the /dev/pts entry to open it by. O_NOCTTY keeps the slave
// from becoming this process's controlling terminal — only the child that is
// spawned with Setctty may claim it.
func openPTY(t *testing.T) (master, slave *os.File) {
	t.Helper()

	mfd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_NONBLOCK, 0)
	if err != nil {
		t.Skipf("no usable /dev/ptmx in this environment: %v", err)
	}
	master = os.NewFile(uintptr(mfd), "/dev/ptmx")
	t.Cleanup(func() { _ = master.Close() })

	if err := unix.IoctlSetPointerInt(mfd, unix.TIOCSPTLCK, 0); err != nil {
		t.Fatalf("unlock pty slave: %v", err)
	}
	n, err := unix.IoctlGetInt(mfd, unix.TIOCGPTN)
	if err != nil {
		t.Fatalf("read pty number: %v", err)
	}

	name := fmt.Sprintf("/dev/pts/%d", n)
	slave, err = os.OpenFile(name, os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		t.Fatalf("open pty slave %s: %v", name, err)
	}
	t.Cleanup(func() { _ = slave.Close() })
	return master, slave
}
