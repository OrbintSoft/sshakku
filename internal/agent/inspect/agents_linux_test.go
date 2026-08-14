//go:build linux

package inspect

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAgentsReadsTheRealProcfs exercises the source an Inspector uses when it
// is given no tree of its own: this machine's /proc. The fake-tree tests prove
// what is made of the bytes; this one proves the bytes are found at all.
//
// It cannot require that any ssh-agent be running — a build machine may have
// none — so it asserts what holds either way: the scan succeeds, and nothing
// it reports is anything but an ssh-agent.
func TestAgentsReadsTheRealProcfs(t *testing.T) {
	procs, err := Inspector{}.Agents()
	require.NoError(t, err, "this machine's own process list must be readable")

	for _, p := range procs {
		require.NotEmptyf(t, p.Args, "pid %d was reported with no argv to have recognised it by", p.PID)
		assert.Equalf(t, "ssh-agent", filepath.Base(p.Args[0]),
			"pid %d is reported as an ssh-agent but was started as %q", p.PID, p.Args[0])
		assert.Positivef(t, p.PID, "the pid reported for %v", p.Args)
	}
}
