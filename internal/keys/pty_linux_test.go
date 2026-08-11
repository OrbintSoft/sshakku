//go:build linux

package keys

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
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

	require.NoError(t, unix.IoctlSetPointerInt(mfd, unix.TIOCSPTLCK, 0), "unlock the pty slave")
	n, err := unix.IoctlGetInt(mfd, unix.TIOCGPTN)
	require.NoError(t, err, "read the pty number")

	name := fmt.Sprintf("/dev/pts/%d", n)
	slave, err = os.OpenFile(name, os.O_RDWR|unix.O_NOCTTY, 0)
	require.NoErrorf(t, err, "open the pty slave %s", name)
	t.Cleanup(func() { _ = slave.Close() })
	return master, slave
}
