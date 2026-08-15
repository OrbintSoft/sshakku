package inspect

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/agent/inspect/inspecttest"
)

// TestReadStatusUIDMalformed covers readStatusUID's fallbacks for a status file
// whose Uid line is missing, has too few fields, or is non-numeric — shapes the
// real /proc never produces, so they are only reachable through a crafted file.
func TestReadStatusUIDMalformed(t *testing.T) {
	write := func(t *testing.T, body string) string {
		t.Helper()
		p := filepath.Join(t.TempDir(), "status")
		require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
		return p
	}

	t.Run("Uid line with too few fields", func(t *testing.T) {
		assert.Equal(t, -1, readStatusUID(write(t, "Name:\tx\nUid:\n")), "a short Uid line leaves the owner unknown")
	})
	t.Run("non-numeric uid", func(t *testing.T) {
		assert.Equal(t, -1, readStatusUID(write(t, "Uid:\tnobody\tnobody\n")), "a non-numeric uid leaves the owner unknown")
	})
	t.Run("no Uid line at all", func(t *testing.T) {
		assert.Equal(t, -1, readStatusUID(write(t, "Name:\tx\nState:\tS\n")), "no Uid line leaves the owner unknown")
	})
}

// TestReadProcfsTreeUnreadableCmdline covers readCmdline's read-failure path: a pid
// directory with no cmdline file is skipped rather than reported as an error.
func TestReadProcfsTreeUnreadableCmdline(t *testing.T) {
	root := t.TempDir()
	// A pid directory that exists but has no cmdline file (the process vanished
	// mid-scan). ReadFile fails and the entry is skipped.
	require.NoError(t, os.Mkdir(filepath.Join(root, "999"), 0o755))
	procs, err := readProcfsTree(root)
	require.NoError(t, err, "readProcfsTree")
	assert.Empty(t, procs, "the cmdline-less entry must be skipped")
}

// TestAgentsChoosesItsSource covers the one decision Agents makes: a caller
// that named a tree gets that tree read, and one that named none gets the
// platform's own source. The two answers are made to differ, because a scan
// that read the wrong source would otherwise still look like a scan.
func TestAgentsChoosesItsSource(t *testing.T) {
	orig := platformSource
	t.Cleanup(func() { platformSource = orig })
	fromPlatform := []AgentProc{{PID: 999, Socket: "/from/platform.sock"}}
	platformSource = func() ([]AgentProc, error) { return fromPlatform, nil }

	root := t.TempDir()
	inspecttest.FakeProc(t, root, 100, []string{"ssh-agent", "-a", "/from/tree.sock"}, 1000)

	named, err := Inspector{ProcRoot: root}.Agents()
	require.NoError(t, err, "a tree the caller named must be readable")
	require.Len(t, named, 1, "the named tree holds one agent")
	assert.Equal(t, "/from/tree.sock", named[0].Socket,
		"a caller that named a tree must be answered from it, not from the machine it happens to run on")

	unnamed, err := Inspector{}.Agents()
	require.NoError(t, err, "the platform's own source must be asked")
	assert.Equal(t, fromPlatform, unnamed,
		"a caller that named no tree must be answered by the platform, whole and unaltered")
}

// TestAgentsReportsWhatThePlatformRefused covers the other half: a platform
// that cannot enumerate says so, and Agents passes that on rather than
// reporting an empty machine — the answer a caller would act on by starting
// an agent that may already be running.
func TestAgentsReportsWhatThePlatformRefused(t *testing.T) {
	orig := platformSource
	t.Cleanup(func() { platformSource = orig })
	refusal := errors.New("this platform cannot enumerate processes")
	platformSource = func() ([]AgentProc, error) { return nil, refusal }

	procs, err := Inspector{}.Agents()
	require.ErrorIs(t, err, refusal, "the platform's refusal must reach the caller")
	assert.Nil(t, procs, "and no process list may come with it")
}
