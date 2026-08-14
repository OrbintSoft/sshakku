package agent

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/agent/inspect"

	"github.com/OrbintSoft/sshakku/internal/agent/inspect/inspecttest"
)

// mapProber reports reachability from a fixed map; absent paths are unreachable.
type mapProber map[string]bool

func (m mapProber) Reachable(_ context.Context, socket string) bool { return m[socket] }

// recordRunner stands in for ssh-agent: it records the socket it was asked to
// start and returns a fixed pid.
type recordRunner struct {
	pid     int
	err     error
	started string
}

func (r *recordRunner) Start(_ context.Context, socket string) (int, error) {
	r.started = socket
	return r.pid, r.err
}

// recordSignaler records the pids it was asked to terminate.
type recordSignaler struct {
	killed []int
}

func (s *recordSignaler) Terminate(pid int) error {
	s.killed = append(s.killed, pid)
	return nil
}

// makeSocketFile leaves a real socket inode at path without auto-unlinking it,
// so removeSocket has a genuine socket to act on.
func makeSocketFile(t *testing.T, path string) {
	t.Helper()
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	require.NoError(t, err)
	l.SetUnlinkOnClose(false)
	_ = l.Close()
}

// shortDir returns a fresh, auto-cleaned temp directory with a short path.
// Unlike t.TempDir(), which nests the (sub)test name under the OS temp root
// (e.g. macOS's /var/folders/xx/.../T/TestName.../001/), it stays well under
// the 104-byte sun_path limit unix sockets are bound under on BSD/Darwin —
// a limit t.TempDir()'s deeper macOS layout routinely exceeds.
func shortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "sshakku") //nolint:usetesting // t.TempDir() is the long macOS path the comment above is about
	require.NoError(t, err, "mkdir temp")
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func TestManagerReap(t *testing.T) {
	root := shortDir(t)
	const ourUID = 1000

	deadOurs := filepath.Join(root, "dead-ours.sock")
	deadOther := filepath.Join(root, "dead-other.sock")
	makeSocketFile(t, deadOurs)
	makeSocketFile(t, deadOther)

	inspecttest.FakeProc(t, root, 100, []string{"ssh-agent", "-a", "/healthy.sock"}, ourUID) // healthy → spare
	inspecttest.FakeProc(t, root, 200, []string{"ssh-agent", "-a", deadOurs}, ourUID)        // dead + ours → reap
	inspecttest.FakeProc(t, root, 300, []string{"ssh-agent", "-a", deadOther}, 1001)         // dead, other user → spare
	inspecttest.FakeProc(t, root, 400, []string{"ssh-agent", "-D"}, ourUID)                  // no socket → spare

	prober := mapProber{"/healthy.sock": true} // everything else is unreachable
	sig := &recordSignaler{}
	m := Manager{Prober: prober, Inspector: inspect.Inspector{ProcRoot: root}, Signaler: sig}

	res, err := m.Reap(t.Context(), ourUID)
	require.NoError(t, err, "Reap")

	assert.Equal(t, []int{200}, sig.killed, "only our own dead agent is terminated")
	assert.Equal(t, []string{deadOurs}, res.RemovedSockets, "only our own dead socket is removed")
	_, err = os.Lstat(deadOurs)
	assert.ErrorIs(t, err, os.ErrNotExist, "our dead socket must be gone")
	_, err = os.Lstat(deadOther)
	assert.NoError(t, err, "another user's socket must be left intact")
}

func TestManagerStart(t *testing.T) {
	dir := shortDir(t)
	socket := filepath.Join(dir, "agent.sock")
	state := filepath.Join(dir, "agent.state")

	// A stale socket sits at the target; the prober says it is unreachable.
	makeSocketFile(t, socket)
	runner := &recordRunner{pid: 4242}
	m := Manager{Prober: mapProber{}, Runner: runner}

	pid, err := m.Start(t.Context(), socket, state)
	require.NoError(t, err, "Start")
	assert.Equal(t, 4242, pid, "the pid the runner announced")
	assert.Equal(t, socket, runner.started, "the socket the runner was asked for")

	got, err := ReadState(state)
	require.NoError(t, err, "ReadState")
	assert.Equal(t, State{PID: 4242, Socket: socket}, got, "the state written for the next shell")
}

func TestStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.state")
	want := State{PID: 99, Socket: "/run/user/1000/sshakku/tok/agent.sock"}
	require.NoError(t, WriteState(path, want))
	got, err := ReadState(path)
	require.NoError(t, err)
	assert.Equal(t, want, got, "the state must survive a round trip")

	_, err = ReadState(filepath.Join(t.TempDir(), "missing"))
	assert.Error(t, err, "ReadState of a missing file must be reported")
}

func TestRemoveSocket(t *testing.T) {
	dir := shortDir(t)
	sock := filepath.Join(dir, "a.sock")
	makeSocketFile(t, sock)
	assert.True(t, removeSocket(sock), "removeSocket must remove a socket")
	_, err := os.Lstat(sock)
	assert.ErrorIs(t, err, os.ErrNotExist, "the socket must be gone")

	reg := filepath.Join(dir, "regular")
	require.NoError(t, os.WriteFile(reg, []byte("x"), 0o600))
	assert.False(t, removeSocket(reg), "removeSocket must refuse a regular file")
	_, err = os.Lstat(reg)
	assert.NoError(t, err, "a regular file must survive")

	assert.False(t, removeSocket(filepath.Join(dir, "missing")), "removeSocket of a missing path reports false")
}

func TestParseAgentPID(t *testing.T) {
	good := "SSH_AUTH_SOCK=/x; export SSH_AUTH_SOCK;\nSSH_AGENT_PID=12345; export SSH_AGENT_PID;\n"
	pid, err := parseAgentPID([]byte(good))
	require.NoError(t, err, "parseAgentPID")
	assert.Equal(t, 12345, pid, "the announced pid")

	_, err = parseAgentPID([]byte("no pid here"))
	assert.Error(t, err, "an absent SSH_AGENT_PID must be reported")

	_, err = parseAgentPID([]byte("SSH_AGENT_PID=;"))
	assert.Error(t, err, "a malformed SSH_AGENT_PID must be reported")
}
