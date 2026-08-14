//go:build unix

package agent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/OrbintSoft/sshakku/internal/agent/inspect"
)

// lockRealAgentTests serialises every real-ssh-agent-spawning test across
// this package AND internal/diagnose (which has its own copy of this
// function) via a flock on a well-known path. `go test ./...` runs different
// packages' test binaries concurrently by default, and these tests kill
// processes by raw pid — without this lock, one package's cleanup can race
// pid reuse against another package's freshly started ssh-agent and kill the
// wrong process (observed in practice as "start ssh-agent: signal:
// terminated").
func lockRealAgentTests(t *testing.T) {
	t.Helper()
	//nolint:usetesting // a per-test directory would give every test its own lock file, which is no lock at all
	f, err := os.OpenFile(filepath.Join(os.TempDir(), "sshakku-test-real-agent.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	require.NoError(t, err, "open cross-package real-agent test lock")
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		_ = f.Close()
		require.NoError(t, err, "flock cross-package real-agent test lock")
	}
	t.Cleanup(func() {
		_ = unix.Flock(int(f.Fd()), unix.LOCK_UN)
		_ = f.Close()
	})
}

// requireIsolatedAgentEnvironment skips the real-process five-state tests
// unless this environment has no ssh-agent already reachable. Inspector scans
// the real, machine-wide /proc, which these tests can't scope down to just
// the processes they spawn — so a pre-existing reachable agent (a real login
// session, or another test suite's leftover) would be picked up as a foreign
// candidate and change which state EnsureAgent lands on. These tests are
// meant for an isolated PID namespace (the container test suite, or a fresh
// CI runner), never a live desktop session.
func requireIsolatedAgentEnvironment(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ssh-agent"); err != nil {
		t.Skip("ssh-agent not on PATH")
	}
	lockRealAgentTests(t)
	procs, err := (inspect.Inspector{}).Agents()
	if err != nil {
		t.Skipf("cannot enumerate /proc: %v", err)
	}
	prober := SocketProber{}
	for _, p := range procs {
		if p.Socket != "" && prober.Reachable(t.Context(), p.Socket) {
			t.Skipf("a real ssh-agent (pid %d, socket %s) is already reachable on this machine — "+
				"these five-state integration tests need an isolated PID namespace (e.g. the "+
				"container test suite), not a live desktop session", p.PID, p.Socket)
		}
	}
}

func newRealManager() Manager {
	return Manager{
		Prober:    SocketProber{},
		Inspector: inspect.Inspector{},
		Runner:    ExecRunner{},
		Signaler:  SysSignaler{},
		Locker:    FlockLocker{Wait: 2 * time.Second},
	}
}

func realCfg(t *testing.T) EnsureConfig {
	t.Helper()
	dir := shortDir(t)
	return EnsureConfig{
		FixedSock: filepath.Join(dir, "agent.sock"),
		LegacyDir: filepath.Join(dir, "legacy"),
		StatePath: filepath.Join(dir, "agent.state"),
		LockPath:  filepath.Join(dir, "agent.lock"),
		OurUID:    os.Getuid(),
	}
}

// stopAgent asks pid to shut down gracefully (SIGTERM), which makes a real
// ssh-agent clean up its own socket, and waits for it to actually exit.
func stopAgent(t *testing.T, pid int) {
	t.Helper()
	if pid == 0 {
		return
	}
	_ = syscall.Kill(pid, syscall.SIGTERM)
	waitGone(t, pid)
}

// killAgentLeavingSocket sends SIGKILL, which an ssh-agent cannot catch, so
// its bound socket file is left behind — reproducing a real crash rather
// than a graceful shutdown, so the stale socket sticks around for the
// "zombie" scenarios to reap.
func killAgentLeavingSocket(t *testing.T, pid int) {
	t.Helper()
	_ = syscall.Kill(pid, syscall.SIGKILL)
	waitGone(t, pid)
}

// waitGone waits for pid to exit — or to become a zombie, which is as gone as
// it gets when nothing in this PID namespace reaps orphans (ssh-agent
// daemonizes via a double fork, so a plain `docker run` with no init process
// never collects it; kill(pid, 0) keeps succeeding for a zombie's still-there
// process slot). Either way it holds no socket and answers nothing, so it
// can't interfere with the next scenario.
func waitGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); err != nil {
			return
		}
		if isZombie(pid) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	require.Failf(t, "a process outlived its deadline", "pid %d did not exit in time", pid)
}

// isZombie reports whether pid is a defunct/zombie process, per its
// /proc/<pid>/status "State:" line.
func isZombie(pid int) bool {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "State:") {
			return strings.Contains(line, "Z (zombie)")
		}
	}
	return false
}

// startForeignAgent starts a real ssh-agent bound to sock, outside the
// Manager entirely — standing in for an agent sshakku did not start (an IDE,
// a desktop session, a manual `ssh-agent -a`).
func startForeignAgent(t *testing.T, sock string) int {
	t.Helper()
	pid, err := (ExecRunner{}).Start(t.Context(), sock)
	require.NoError(t, err, "start foreign ssh-agent")
	t.Cleanup(func() { stopAgent(t, pid) })
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if (SocketProber{}).Reachable(t.Context(), sock) {
			return pid
		}
		time.Sleep(20 * time.Millisecond)
	}
	require.Failf(t, "a foreign agent never came up", "pid %d never became reachable on %s", pid, sock)
	return pid
}

func TestEnsureAgentRealClean(t *testing.T) {
	requireIsolatedAgentEnvironment(t)
	m := newRealManager()
	cfg := realCfg(t)

	res, err := m.EnsureAgent(t.Context(), cfg, nil)
	require.NoError(t, err, "EnsureAgent")
	t.Cleanup(func() { stopAgent(t, res.Started) })

	assert.Equal(t, SituationClean, res.Situation, "situation")
	assert.NotZero(t, res.Started, "a pid must have been started")
	assert.True(t, m.Prober.Reachable(t.Context(), cfg.FixedSock), "the fixed socket must be reachable after a clean start")
}

func TestEnsureAgentRealHealthyReuse(t *testing.T) {
	requireIsolatedAgentEnvironment(t)
	m := newRealManager()
	cfg := realCfg(t)

	res1, err := m.EnsureAgent(t.Context(), cfg, nil)
	require.NoError(t, err, "first EnsureAgent")
	t.Cleanup(func() { stopAgent(t, res1.Started) })

	res2, err := m.EnsureAgent(t.Context(), cfg, nil)
	require.NoError(t, err, "second EnsureAgent")
	assert.Equal(t, SituationHealthy, res2.Situation, "situation")
	assert.Zero(t, res2.Started, "no new agent must be started on reuse")
}

// TestEnsureAgentRealReachableButEmptyIsHealthy covers the D1 case: an
// agent with zero keys loaded (ssh-add -l exits 1) must still be treated as
// healthy, never killed and replaced.
func TestEnsureAgentRealReachableButEmptyIsHealthy(t *testing.T) {
	requireIsolatedAgentEnvironment(t)
	m := newRealManager()
	cfg := realCfg(t)

	res1, err := m.EnsureAgent(t.Context(), cfg, nil)
	require.NoError(t, err, "first EnsureAgent")
	t.Cleanup(func() { stopAgent(t, res1.Started) })

	// No keys were ever added, so the agent's own reply to
	// SSH_AGENTC_REQUEST_IDENTITIES lists zero identities — the real-world
	// equivalent of `ssh-add -l` exiting 1. SocketProber's handshake already
	// exercises exactly that round trip, so a second probe here is redundant;
	// the point being tested is that EnsureAgent still calls this healthy.

	res2, err := m.EnsureAgent(t.Context(), cfg, nil)
	require.NoError(t, err, "second EnsureAgent")
	assert.Equal(t, SituationHealthy, res2.Situation, "an empty agent is still healthy")
	assert.Zero(t, res2.Started, "an empty-but-reachable agent must never be replaced")
}

func TestEnsureAgentRealZombie(t *testing.T) {
	requireIsolatedAgentEnvironment(t)
	m := newRealManager()
	cfg := realCfg(t)

	res1, err := m.EnsureAgent(t.Context(), cfg, nil)
	require.NoError(t, err, "first EnsureAgent")
	killAgentLeavingSocket(t, res1.Started)
	require.False(t, m.Prober.Reachable(t.Context(), cfg.FixedSock), "the socket must be dead after SIGKILL")

	res2, err := m.EnsureAgent(t.Context(), cfg, nil)
	require.NoError(t, err, "second EnsureAgent")
	t.Cleanup(func() { stopAgent(t, res2.Started) })

	assert.Equal(t, SituationZombie, res2.Situation, "situation")
	assert.False(t, len(res2.Reaped.Terminated) == 0 && len(res2.Reaped.RemovedSockets) == 0,
		"the dead agent or its socket must have been reaped")
	assert.True(t, m.Prober.Reachable(t.Context(), cfg.FixedSock), "a fresh healthy agent must answer after the zombie reap")
}

// TestEnsureAgentRealGracefulStopRemovesSocket is the graceful counterpart to
// TestEnsureAgentRealZombie: SIGTERM (unlike SIGKILL) is caught by ssh-agent,
// whose signal handler unlinks the socket it was started on before exiting. So
// a graceful stop leaves no stale socket behind, and the next EnsureAgent is a
// clean start with nothing to reap — not a zombie reap.
func TestEnsureAgentRealGracefulStopRemovesSocket(t *testing.T) {
	requireIsolatedAgentEnvironment(t)
	m := newRealManager()
	cfg := realCfg(t)

	res1, err := m.EnsureAgent(t.Context(), cfg, nil)
	require.NoError(t, err, "first EnsureAgent")
	require.Equal(t, SituationClean, res1.Situation, "setup situation")
	require.True(t, m.Prober.Reachable(t.Context(), cfg.FixedSock), "the fixed socket must be reachable after a clean start")

	// SIGTERM, not SIGKILL: ssh-agent catches it and unlinks its own socket.
	stopAgent(t, res1.Started)

	require.False(t, m.Prober.Reachable(t.Context(), cfg.FixedSock), "the socket must be dead after SIGTERM")
	_, err = os.Lstat(cfg.FixedSock)
	assert.ErrorIs(t, err, os.ErrNotExist, "a graceful SIGTERM must leave the socket unlinked, not stale")

	// With no stale socket to reap, the next EnsureAgent is a clean start.
	res2, err := m.EnsureAgent(t.Context(), cfg, nil)
	require.NoError(t, err, "second EnsureAgent")
	t.Cleanup(func() { stopAgent(t, res2.Started) })

	assert.Equal(t, SituationClean, res2.Situation, "with no stale socket to reap this is a clean start")
	assert.Empty(t, res2.Reaped.RemovedSockets, "nothing must have been removed after a graceful stop")
	assert.Empty(t, res2.Reaped.Terminated, "nothing must have been terminated after a graceful stop")
}

// TestEnsureAgentRealForeignAdopted covers state D: a healthy agent sshakku
// did not start must be adopted via the fixed-socket symlink, not killed.
func TestEnsureAgentRealForeignAdopted(t *testing.T) {
	requireIsolatedAgentEnvironment(t)
	m := newRealManager()
	cfg := realCfg(t)

	foreignSock := filepath.Join(shortDir(t), "foreign.sock")
	foreignPID := startForeignAgent(t, foreignSock)

	res, err := m.EnsureAgent(t.Context(), cfg, nil)
	require.NoError(t, err, "EnsureAgent")

	assert.Equal(t, SituationForeign, res.Situation, "situation")
	assert.Zero(t, res.Started, "a competing agent must never be started when a healthy foreign one exists")
	require.NotNil(t, res.Adopted, "an agent must have been adopted")
	assert.Equal(t, foreignPID, res.Adopted.PID, "the adopted pid")
	assert.NotEmpty(t, res.Anomaly, "adopting a foreign agent must report an anomaly")
	assert.True(t, m.Prober.Reachable(t.Context(), cfg.FixedSock), "the fixed socket must reach the adopted foreign agent")
	// The foreign agent itself must still be alive — never killed.
	assert.NoError(t, syscall.Kill(foreignPID, 0), "the foreign agent must be left running")
}

// TestEnsureAgentRealDisasterReapsAndAdoptsLowestPID covers state E: a dead
// agent of ours plus two healthy foreign agents. EnsureAgent must reap the
// dead one, adopt the lowest-pid healthy foreign one, and report disaster.
func TestEnsureAgentRealDisasterReapsAndAdoptsLowestPID(t *testing.T) {
	requireIsolatedAgentEnvironment(t)
	m := newRealManager()
	cfg := realCfg(t)

	res1, err := m.EnsureAgent(t.Context(), cfg, nil)
	require.NoError(t, err, "seed EnsureAgent")
	killAgentLeavingSocket(t, res1.Started) // now dead-ours

	sockA := filepath.Join(shortDir(t), "foreign-a.sock")
	sockB := filepath.Join(shortDir(t), "foreign-b.sock")
	pidA := startForeignAgent(t, sockA)
	pidB := startForeignAgent(t, sockB)
	lowest := pidA
	if pidB < pidA {
		lowest = pidB
	}

	res2, err := m.EnsureAgent(t.Context(), cfg, nil)
	require.NoError(t, err, "EnsureAgent")

	assert.Equal(t, SituationDisaster, res2.Situation, "situation")
	assert.NotEmpty(t, res2.Reaped.RemovedSockets, "the dead-ours socket must have been reaped")
	require.NotNil(t, res2.Adopted, "an agent must have been adopted")
	assert.Equal(t, lowest, res2.Adopted.PID, "the lowest-pid healthy foreign agent must be adopted")
	for _, pid := range []int{pidA, pidB} {
		assert.NoErrorf(t, syscall.Kill(pid, 0), "foreign agent pid %d must be left running", pid)
	}
}
