// Package inspecttest builds the procfs-shaped directory trees inspect reads,
// so a test can decide what processes were running instead of asking the
// machine it happens to run on.
//
// It writes plain files and knows nothing of inspect itself, which is what
// lets it serve inspect's own tests and the agent lifecycle's alike, and lets
// both run on an operating system that has no /proc.
package inspecttest

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// FakeProc writes a /proc/<pid> entry with the given argv and (optional) status
// Uid line into root. A negative uid omits the status file, simulating a process
// whose owner we cannot read.
func FakeProc(t *testing.T, root string, pid int, argv []string, uid int) {
	t.Helper()
	dir := filepath.Join(root, strconv.Itoa(pid))
	require.NoError(t, os.MkdirAll(dir, 0o755))
	cmdline := strings.Join(argv, "\x00")
	if len(argv) > 0 {
		cmdline += "\x00" // the kernel NUL-terminates the final arg too.
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cmdline"), []byte(cmdline), 0o644))
	if uid >= 0 {
		status := "Name:\tssh-agent\nUid:\t" + strconv.Itoa(uid) + "\t" + strconv.Itoa(uid) + "\t" + strconv.Itoa(uid) + "\t" + strconv.Itoa(uid) + "\n"
		require.NoError(t, os.WriteFile(filepath.Join(dir, "status"), []byte(status), 0o644))
	}
}
