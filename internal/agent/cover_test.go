package agent

import (
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
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
	t.Run("write fails", func(t *testing.T) {
		if identitiesAnswered(&errReadWriter{writeErr: errors.New("broken pipe")}) {
			t.Error("want false when the request write fails")
		}
	})
	t.Run("short header", func(t *testing.T) {
		// Fewer than 4 header bytes: io.ReadFull returns before the frame is read.
		if identitiesAnswered(&errReadWriter{readBuf: []byte{0, 0}}) {
			t.Error("want false when the length header is truncated")
		}
	})
	t.Run("zero length frame", func(t *testing.T) {
		if identitiesAnswered(&errReadWriter{readBuf: []byte{0, 0, 0, 0}}) {
			t.Error("want false when the framed length is below 1")
		}
	})
	t.Run("oversized length frame", func(t *testing.T) {
		// length = maxFrame+1, above the cap.
		hdr := make([]byte, 4)
		binary.BigEndian.PutUint32(hdr, maxFrame+1)
		if identitiesAnswered(&errReadWriter{readBuf: hdr}) {
			t.Error("want false when the framed length exceeds the cap")
		}
	})
	t.Run("type byte truncated", func(t *testing.T) {
		// A valid length of 1 but no message-type byte follows: the second
		// io.ReadFull hits EOF.
		if identitiesAnswered(&errReadWriter{readBuf: []byte{0, 0, 0, 1}}) {
			t.Error("want false when the message type is missing")
		}
	})
}

// TestReadStatusUIDMalformed covers readStatusUID's fallbacks for a status file
// whose Uid line is missing, has too few fields, or is non-numeric — shapes the
// real /proc never produces, so they are only reachable through a crafted file.
func TestReadStatusUIDMalformed(t *testing.T) {
	write := func(t *testing.T, body string) string {
		t.Helper()
		p := filepath.Join(t.TempDir(), "status")
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}

	t.Run("Uid line with too few fields", func(t *testing.T) {
		if got := readStatusUID(write(t, "Name:\tx\nUid:\n")); got != -1 {
			t.Errorf("readStatusUID = %d, want -1 for a short Uid line", got)
		}
	})
	t.Run("non-numeric uid", func(t *testing.T) {
		if got := readStatusUID(write(t, "Uid:\tnobody\tnobody\n")); got != -1 {
			t.Errorf("readStatusUID = %d, want -1 for a non-numeric uid", got)
		}
	})
	t.Run("no Uid line at all", func(t *testing.T) {
		if got := readStatusUID(write(t, "Name:\tx\nState:\tS\n")); got != -1 {
			t.Errorf("readStatusUID = %d, want -1 when no Uid line is present", got)
		}
	})
}

// TestSituationStringUnknown covers Situation.String's default arm for a value
// outside the defined set.
func TestSituationStringUnknown(t *testing.T) {
	if got := Situation(99).String(); got != "unknown" {
		t.Errorf("Situation(99).String() = %q, want %q", got, "unknown")
	}
}

// TestReadProcfsTreeUnreadableCmdline covers readCmdline's read-failure path: a pid
// directory with no cmdline file is skipped rather than reported as an error.
func TestReadProcfsTreeUnreadableCmdline(t *testing.T) {
	root := t.TempDir()
	// A pid directory that exists but has no cmdline file (the process vanished
	// mid-scan). ReadFile fails and the entry is skipped.
	if err := os.Mkdir(filepath.Join(root, "999"), 0o755); err != nil {
		t.Fatal(err)
	}
	procs, err := readProcfsTree(root)
	if err != nil {
		t.Fatalf("readProcfsTree: %v", err)
	}
	if len(procs) != 0 {
		t.Errorf("got %d procs, want none (the cmdline-less entry is skipped)", len(procs))
	}
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
	if _, err := m.EnsureAgent(EnsureConfig{FixedSock: fixed, OurUID: 1000}, nil); err == nil {
		t.Fatal("want an error when reaping cannot read the process list")
	}
}

// TestManagerReapInspectError covers Reap's error return when the process list
// cannot be read (a missing procfs root).
func TestManagerReapInspectError(t *testing.T) {
	m := Manager{Inspector: Inspector{ProcRoot: filepath.Join(t.TempDir(), "nope")}}
	if _, err := m.Reap(1000); err == nil {
		t.Fatal("want an error when the process list cannot be read")
	}
}

// TestManagerStartRunnerError covers Start's error return when the runner fails to
// launch an agent — the state file must not be written.
func TestManagerStartRunnerError(t *testing.T) {
	dir := shortDir(t)
	socket := filepath.Join(dir, "agent.sock")
	state := filepath.Join(dir, "agent.state")
	m := Manager{Prober: mapProber{}, Runner: &recordRunner{err: errors.New("no ssh-agent")}}

	if _, err := m.Start(socket, state); err == nil {
		t.Fatal("want an error when the runner fails")
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Errorf("no state file should be written on a runner failure, stat err = %v", err)
	}
}

// TestManagerStartStateWriteError covers Start's non-fatal path: the agent came up
// but recording its state failed. It must still return the pid alongside the error.
func TestManagerStartStateWriteError(t *testing.T) {
	dir := shortDir(t)
	socket := filepath.Join(dir, "agent.sock")
	state := filepath.Join(dir, "no-such-dir", "agent.state") // parent missing → write fails
	m := Manager{Prober: mapProber{}, Runner: &recordRunner{pid: 4242}}

	pid, err := m.Start(socket, state)
	if err == nil {
		t.Fatal("want an error when the state file cannot be written")
	}
	if pid != 4242 {
		t.Errorf("pid = %d, want the started agent's pid 4242 even when state recording fails", pid)
	}
}

// TestWriteStateError covers WriteState's os.WriteFile failure path.
func TestWriteStateError(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "no-such-dir", "agent.state")
	if err := WriteState(bad, State{PID: 1, Socket: "/x"}); err == nil {
		t.Fatal("want an error writing state under a missing directory")
	}
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
	res, err := m.EnsureAgent(EnsureConfig{FixedSock: fixed, StatePath: filepath.Join(dir, "st"), OurUID: 1000}, log)
	if err != nil {
		t.Fatal(err)
	}
	if res.Situation != SituationZombie {
		t.Fatalf("situation = %v, want zombie after clearing an orphan socket", res.Situation)
	}
	if !containsStr(res.Reaped.RemovedSockets, fixed) {
		t.Errorf("removed sockets = %v, want the orphan %s", res.Reaped.RemovedSockets, fixed)
	}
	if runner.started != fixed {
		t.Errorf("started %q, want a fresh agent on %q", runner.started, fixed)
	}
}

// containsStr reports membership for the socket-path slices ReapResult uses.
func containsStr(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
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
	if _, err := m.EnsureAgent(EnsureConfig{FixedSock: fixed, StatePath: filepath.Join(dir, "st"), OurUID: 1000}, nil); err == nil {
		t.Fatal("want an error when starting the agent fails")
	}
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
	res, err := m.EnsureAgent(EnsureConfig{FixedSock: fixed, OurUID: 1000}, log)
	if err != nil {
		t.Fatal(err)
	}
	if res.Situation != SituationDisaster {
		t.Fatalf("situation = %v, want disaster (adopt after replacing a stale socket)", res.Situation)
	}
	if !containsStr(res.Reaped.RemovedSockets, fixed) {
		t.Errorf("removed sockets = %v, want the replaced fixed socket %s", res.Reaped.RemovedSockets, fixed)
	}
	if target, _ := os.Readlink(fixed); target != foreignSock {
		t.Errorf("fixed now points at %q, want the adopted %q", target, foreignSock)
	}
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
	if _, err := m.EnsureAgent(EnsureConfig{FixedSock: fixed, OurUID: 1000}, nil); err == nil {
		t.Fatal("want an error when adoption cannot symlink the fixed socket")
	}
}

// TestAdoptSymlinkErrors covers adoptSymlink's two failure returns directly: the
// symlink cannot be created (missing parent directory), and the atomic rename onto
// the fixed path fails (the target path is an existing directory).
func TestAdoptSymlinkErrors(t *testing.T) {
	dir := shortDir(t)

	t.Run("symlink fails", func(t *testing.T) {
		fixed := filepath.Join(dir, "no-such-dir", "agent.sock")
		if err := adoptSymlink(fixed, "/some/target"); err == nil {
			t.Error("want an error when the symlink cannot be created")
		}
	})
	t.Run("rename fails", func(t *testing.T) {
		occupied := filepath.Join(dir, "occupied")
		if err := os.Mkdir(occupied, 0o755); err != nil {
			t.Fatal(err)
		}
		// Renaming the temp symlink onto an existing directory fails.
		if err := adoptSymlink(occupied, "/some/target"); err == nil {
			t.Error("want an error when the rename onto the fixed path fails")
		}
		if _, err := os.Lstat(occupied + ".adopt"); !os.IsNotExist(err) {
			t.Errorf("the temp symlink must be cleaned up on a rename failure, lstat err = %v", err)
		}
	})
}
