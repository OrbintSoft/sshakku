package sessionlog

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/flock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// What a re-executed test binary reads to know it is one of the writers rather
// than the test run itself. The environment carries it because the child is
// started with no arguments of its own: anything passed there would have to get
// past the testing package's own flag parsing.
const (
	writerLogEnv   = "SSHAKKU_TEST_SESSIONLOG_PATH"
	writerTagEnv   = "SSHAKKU_TEST_SESSIONLOG_TAG"
	writerLinesEnv = "SSHAKKU_TEST_SESSIONLOG_LINES"
	writerMaxEnv   = "SSHAKKU_TEST_SESSIONLOG_MAX"
)

// TestMain answers as a writer when asked to be one, and otherwise runs the
// tests. The check comes first because the testing package would reject the
// child's environment-borne instructions as unknown flags.
func TestMain(m *testing.M) {
	if path := os.Getenv(writerLogEnv); path != "" {
		os.Exit(writeAsChild(path))
	}
	os.Exit(m.Run())
}

// writeAsChild is one login shell's worth of logging, done through the same
// Logger the product uses. Nothing here is a stand-in: what the test is asking
// about is what this type does when another process is doing it too.
func writeAsChild(path string) int {
	lines, err := strconv.Atoi(os.Getenv(writerLinesEnv))
	if err != nil {
		return 2
	}
	maxLines, err := strconv.Atoi(os.Getenv(writerMaxEnv))
	if err != nil {
		return 2
	}
	// Built through New so the child queues for the lock on the same terms the
	// product does; only the cap is the test's to choose.
	logger := New(path)
	logger.maxLines = maxLines
	tag := os.Getenv(writerTagEnv)
	for i := range lines {
		if err := logger.Log("INFO", fmt.Sprintf("%s-%d", tag, i)); err != nil {
			return 1
		}
	}
	return 0
}

// TestLogKeepsEveryLineWrittenConcurrently verifies F12: shells opening at the
// same moment each get their lines into the log, and none is lost for having
// been written while another session was writing too.
//
// It drives the log the way a burst of simultaneous logins does: several
// processes appending to one file that is already at its line cap, so that
// every single write also trims.
//
// The writers are real processes because that is what the case is. Two login
// shells are two programs, each holding its own Logger, and a defect in how they
// share the file is invisible to anything they could share in memory — a version
// of this test using goroutines would pass against an in-process mutex while two
// shells went on losing each other's lines.
//
// Fewer lines are written than the cap allows, so trimming has no line of its own
// to drop: every line written must still be there at the end. Counting them would
// not have asked this. A writer that rewrites the file over another's append
// leaves it holding exactly maxLines lines, and the file looks healthy; what is
// wrong is which lines they are.
func TestLogKeepsEveryLineWrittenConcurrently(t *testing.T) {
	const (
		writers   = 8
		perWriter = 40
		maxLines  = 500
	)

	path := filepath.Join(t.TempDir(), "sessions.log")

	// Start at the cap, so the first write of every writer already trims.
	var filler strings.Builder
	for i := range maxLines {
		_, _ = fmt.Fprintf(&filler, "%s | [INFO] filler-%d\n", timeLayout, i)
	}
	require.NoError(t, os.WriteFile(path, []byte(filler.String()), filePerm))

	exe, err := os.Executable()
	require.NoError(t, err, "locating this test binary, which is the program the writers run")

	started := make([]*exec.Cmd, 0, writers)
	for w := range writers {
		cmd := exec.CommandContext(t.Context(), exe)
		cmd.Env = append(os.Environ(),
			writerLogEnv+"="+path,
			writerTagEnv+"="+fmt.Sprintf("writer%d", w),
			writerLinesEnv+"="+strconv.Itoa(perWriter),
			writerMaxEnv+"="+strconv.Itoa(maxLines),
		)
		require.NoErrorf(t, cmd.Start(), "starting writer %d", w)
		started = append(started, cmd)
	}
	for w, cmd := range started {
		assert.NoErrorf(t, cmd.Wait(), "writer %d", w)
	}

	data, err := os.ReadFile(path)
	require.NoError(t, err, "reading the log the writers shared")
	got := string(data)

	var lost []string
	for w := range writers {
		for i := range perWriter {
			if line := fmt.Sprintf("writer%d-%d", w, i); !strings.Contains(got, line+"\n") {
				lost = append(lost, line)
			}
		}
	}
	assert.Emptyf(t, lost, "%d of %d lines written are not in the log", len(lost), writers*perWriter)

	kept := strings.Split(strings.TrimRight(got, "\n"), "\n")
	assert.Len(t, kept, maxLines, "the log stays at its cap")
}

// TestLogUnderAHeldLockRecordsWithoutTrimming covers F12 from the side where the
// promise is hardest to keep: the lock is not available at all, and the line
// still has to be recorded.
//
// A writer that gives up on the lock appends anyway, because an unlocked append
// is what the old code did to every line and it loses one only if a trim happens
// to be rewriting the file just then. What it must not do is trim: that is the
// half that overwrites, and doing it while somebody else holds the lock is
// precisely the damage the lock was taken out against.
func TestLogUnderAHeldLockRecordsWithoutTrimming(t *testing.T) {
	const lines = 5

	path := filepath.Join(t.TempDir(), "sessions.log")
	logger := New(path)
	// A cap far below what is about to be written, so a trim that ran would be
	// impossible to miss.
	logger.maxLines = 2
	logger.lockWait = 20 * time.Millisecond
	logger.lockPoll = 5 * time.Millisecond

	// Somebody else holds the lock, and goes on holding it throughout.
	other := flock.New(path + lockSuffix)
	taken, err := other.TryLock()
	require.NoError(t, err, "taking the lock this test holds against the logger")
	require.True(t, taken, "the lock has to start free for this to be a test of contention")
	t.Cleanup(func() { _ = other.Unlock() })

	for i := range lines {
		require.NoErrorf(t, logger.Log("INFO", fmt.Sprintf("line-%d", i)), "line %d", i)
	}

	data, err := os.ReadFile(path)
	require.NoError(t, err, "reading the log")
	got := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	assert.Len(t, got, lines, "every line is recorded, including the ones written unlocked")
	assert.Contains(t, got[0], "line-0", "nothing was trimmed away while another writer held the lock")
}
