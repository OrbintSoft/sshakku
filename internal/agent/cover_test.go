package agent

import (
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errReadWriter is an in-process io.ReadWriter that fails Write with writeErr (when
// set), otherwise serves reads from a fixed buffer. It lets identitiesAnswered be
// exercised directly, without a real socket, for the framing edge cases.
type errReadWriter struct {
	writeErr error
	readBuf  []byte
}

func (e *errReadWriter) Write(p []byte) (int, error) {
	if e.writeErr != nil {
		return 0, e.writeErr
	}
	return len(p), nil
}

func (e *errReadWriter) Read(p []byte) (int, error) {
	if len(e.readBuf) == 0 {
		return 0, io.EOF
	}
	n := copy(p, e.readBuf)
	e.readBuf = e.readBuf[n:]
	return n, nil
}

// TestIdentitiesAnsweredEdges covers the write-failure and malformed-frame branches
// of identitiesAnswered that a healthy fake agent never triggers.
func TestIdentitiesAnsweredEdges(t *testing.T) {
	// Each buffer below carries a message type that would be accepted, so the
	// only thing that can make these false is the check being tested. Left
	// empty, the read simply runs out and every one of them would pass with the
	// check deleted.
	answer := []byte{msgIdentitiesAnswer}

	t.Run("write fails", func(t *testing.T) {
		wellFormed := append([]byte{0, 0, 0, 1}, answer...)
		assert.False(t, identitiesAnswered(&errReadWriter{writeErr: errors.New("broken pipe"), readBuf: wellFormed}),
			"a request that could not be written must not be believed answered")
	})
	t.Run("short header", func(t *testing.T) {
		// Fewer than 4 header bytes: io.ReadFull returns before the frame is read.
		assert.False(t, identitiesAnswered(&errReadWriter{readBuf: []byte{0, 0}}),
			"a truncated length header is not an answer")
	})
	t.Run("zero length frame", func(t *testing.T) {
		assert.False(t, identitiesAnswered(&errReadWriter{readBuf: append([]byte{0, 0, 0, 0}, answer...)}),
			"a framed length below 1 is not an answer, whatever byte follows it")
	})
	t.Run("oversized length frame", func(t *testing.T) {
		// length = maxFrame+1, above the cap.
		hdr := make([]byte, 4)
		binary.BigEndian.PutUint32(hdr, maxFrame+1)
		assert.False(t, identitiesAnswered(&errReadWriter{readBuf: append(hdr, answer...)}),
			"a framed length above the cap is not an answer, whatever byte follows it")
	})
	t.Run("type byte truncated", func(t *testing.T) {
		// A valid length of 1 but no message-type byte follows: the second
		// io.ReadFull hits EOF.
		assert.False(t, identitiesAnswered(&errReadWriter{readBuf: []byte{0, 0, 0, 1}}),
			"a missing message type is not an answer")
	})
}

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

// TestSituationStringUnknown covers Situation.String's default arm for a value
// outside the defined set.
func TestSituationStringUnknown(t *testing.T) {
	assert.Equal(t, "unknown", Situation(99).String(), "a situation outside the defined set")
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

// TestEnsureAgentReapError covers EnsureAgent's error return when the reap pass
// cannot enumerate processes (a missing procfs root).
func TestEnsureAgentReapError(t *testing.T) {
	dir := shortDir(t)
	fixed := filepath.Join(dir, "agent.sock")
	m := Manager{
		Prober:    mapProber{}, // fixed silent → past the fast path
		Inspector: Inspector{ProcRoot: filepath.Join(dir, "nope")},
		Runner:    &recordRunner{},
		Signaler:  &recordSignaler{},
	}
	_, err := m.EnsureAgent(t.Context(), EnsureConfig{FixedSock: fixed, OurUID: 1000}, nil)
	assert.Error(t, err, "a reap that cannot read the process list must be reported")
}

// TestManagerReapInspectError covers Reap's error return when the process list
// cannot be read (a missing procfs root).
func TestManagerReapInspectError(t *testing.T) {
	m := Manager{Inspector: Inspector{ProcRoot: filepath.Join(t.TempDir(), "nope")}}
	_, err := m.Reap(t.Context(), 1000)
	assert.Error(t, err, "a process list that cannot be read must be reported")
}

// TestManagerStartRunnerError covers Start's error return when the runner fails to
// launch an agent — the state file must not be written.
func TestManagerStartRunnerError(t *testing.T) {
	dir := shortDir(t)
	socket := filepath.Join(dir, "agent.sock")
	state := filepath.Join(dir, "agent.state")
	m := Manager{Prober: mapProber{}, Runner: &recordRunner{err: errors.New("no ssh-agent")}}

	_, err := m.Start(t.Context(), socket, state)
	assert.Error(t, err, "a runner that fails must be reported")
	_, err = os.Stat(state)
	assert.ErrorIs(t, err, os.ErrNotExist, "no state file must be written on a runner failure")
}

// TestManagerStartStateWriteError covers Start's non-fatal path: the agent came up
// but recording its state failed. It must still return the pid alongside the error.
func TestManagerStartStateWriteError(t *testing.T) {
	dir := shortDir(t)
	socket := filepath.Join(dir, "agent.sock")
	state := filepath.Join(dir, "no-such-dir", "agent.state") // parent missing → write fails
	m := Manager{Prober: mapProber{}, Runner: &recordRunner{pid: 4242}}

	pid, err := m.Start(t.Context(), socket, state)
	assert.Error(t, err, "a state file that cannot be written must be reported")
	assert.Equal(t, 4242, pid, "the started agent's pid must come back even when recording its state fails")
}

// TestWriteStateError covers WriteState's os.WriteFile failure path.
func TestWriteStateError(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "no-such-dir", "agent.state")
	assert.Error(t, WriteState(bad, State{PID: 1, Socket: "/x"}),
		"writing state under a missing directory must be reported")
}

// TestEnsureAgentClearsStaleFixedSocket covers the start path's stale-socket
// removal: a socket sits at the fixed path with no process owning it (Reap never
// saw one), so EnsureAgent clears it before starting a fresh agent and reports a
// zombie recovery rather than a clean start.
func TestEnsureAgentClearsStaleFixedSocket(t *testing.T) {
	dir := shortDir(t)
	fixed := filepath.Join(dir, "agent.sock")
	makeSocketFile(t, fixed) // orphan socket, no matching proc

	runner := &recordRunner{pid: 7000}
	log := &fakeLogger{}
	m := Manager{
		Prober:    mapProber{},                      // fixed is silent
		Inspector: Inspector{ProcRoot: shortDir(t)}, // no processes at all
		Runner:    runner,
		Signaler:  &recordSignaler{},
	}
	res, err := m.EnsureAgent(t.Context(), EnsureConfig{FixedSock: fixed, StatePath: filepath.Join(dir, "st"), OurUID: 1000}, log)
	require.NoError(t, err)
	assert.Equal(t, SituationZombie, res.Situation, "clearing an orphan socket is a zombie recovery, not a clean start")
	assert.Contains(t, res.Reaped.RemovedSockets, fixed, "the orphan socket must be reported as removed")
	assert.Equal(t, fixed, runner.started, "a fresh agent must be started on the fixed socket")
}

// TestEnsureAgentStartError covers EnsureAgent's error return when starting the
// fresh agent fails in the no-foreign branch.
func TestEnsureAgentStartError(t *testing.T) {
	dir := shortDir(t)
	fixed := filepath.Join(dir, "agent.sock")
	m := Manager{
		Prober:    mapProber{},
		Inspector: Inspector{ProcRoot: shortDir(t)},
		Runner:    &recordRunner{err: errors.New("no ssh-agent")},
		Signaler:  &recordSignaler{},
	}
	_, err := m.EnsureAgent(t.Context(), EnsureConfig{FixedSock: fixed, StatePath: filepath.Join(dir, "st"), OurUID: 1000}, nil)
	assert.Error(t, err, "an agent that cannot be started must be reported")
}

// TestEnsureAgentReplacesStaleFixedOnAdopt covers the adopt path's stale-socket
// notice: a healthy foreign agent exists and an orphan socket sits at the fixed
// path (no process owns it, so Reap left it). adoptSymlink will replace it, and
// EnsureAgent reports the replacement, escalating the landscape to a disaster.
func TestEnsureAgentReplacesStaleFixedOnAdopt(t *testing.T) {
	dir := shortDir(t)
	fixed := filepath.Join(dir, "agent.sock")
	proc := shortDir(t)
	foreignSock := filepath.Join(dir, "foreign.sock")

	makeSocketFile(t, fixed)                                               // orphan socket at the fixed path
	fakeProc(t, proc, 300, []string{"ssh-agent", "-a", foreignSock}, 1000) // healthy foreign, unrelated socket

	log := &fakeLogger{}
	m := Manager{
		Prober:    mapProber{foreignSock: true}, // fixed silent, foreign healthy
		Inspector: Inspector{ProcRoot: proc},
		Runner:    &recordRunner{},
		Signaler:  &recordSignaler{},
	}
	res, err := m.EnsureAgent(t.Context(), EnsureConfig{FixedSock: fixed, OurUID: 1000}, log)
	require.NoError(t, err)
	assert.Equal(t, SituationDisaster, res.Situation, "adopting after replacing a stale socket is a disaster")
	assert.Contains(t, res.Reaped.RemovedSockets, fixed, "the replaced fixed socket must be reported as removed")

	target, _ := os.Readlink(fixed)
	assert.Equal(t, foreignSock, target, "the fixed path must point at the adopted socket")
}

// TestEnsureAgentAdoptSymlinkError covers EnsureAgent's error return when adopting
// a foreign agent fails because the fixed socket's directory does not exist.
func TestEnsureAgentAdoptSymlinkError(t *testing.T) {
	dir := shortDir(t)
	fixed := filepath.Join(dir, "no-such-dir", "agent.sock") // parent missing → symlink fails
	proc := shortDir(t)
	foreignSock := filepath.Join(dir, "foreign.sock")
	fakeProc(t, proc, 300, []string{"ssh-agent", "-a", foreignSock}, 1000)

	m := Manager{
		Prober:    mapProber{foreignSock: true},
		Inspector: Inspector{ProcRoot: proc},
		Runner:    &recordRunner{},
		Signaler:  &recordSignaler{},
	}
	_, err := m.EnsureAgent(t.Context(), EnsureConfig{FixedSock: fixed, OurUID: 1000}, nil)
	assert.Error(t, err, "an adoption that cannot symlink the fixed socket must be reported")
}

// TestAdoptSymlinkErrors covers adoptSymlink's two failure returns directly: the
// symlink cannot be created (missing parent directory), and the atomic rename onto
// the fixed path fails (the target path is an existing directory).
func TestAdoptSymlinkErrors(t *testing.T) {
	dir := shortDir(t)

	t.Run("symlink fails", func(t *testing.T) {
		fixed := filepath.Join(dir, "no-such-dir", "agent.sock")
		assert.Error(t, adoptSymlink(fixed, "/some/target"), "a symlink that cannot be created must be reported")
	})
	t.Run("rename fails", func(t *testing.T) {
		occupied := filepath.Join(dir, "occupied")
		require.NoError(t, os.Mkdir(occupied, 0o755))
		// Renaming the temp symlink onto an existing directory fails.
		assert.Error(t, adoptSymlink(occupied, "/some/target"), "a rename onto the fixed path that fails must be reported")
		_, err := os.Lstat(occupied + ".adopt")
		assert.ErrorIs(t, err, os.ErrNotExist, "the temp symlink must be cleaned up on a rename failure")
	})
}
