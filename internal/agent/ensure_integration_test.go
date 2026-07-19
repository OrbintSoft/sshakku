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

	"golang.org/x/sys/unix"
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
	f, err := os.OpenFile(filepath.Join(os.TempDir(), "sshakku-test-real-agent.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("open cross-package real-agent test lock: %v", err)
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		_ = f.Close()
		t.Fatalf("flock cross-package real-agent test lock: %v", err)
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
// meant for an isolated PID namespace (the tier-1 container, or a fresh CI
// runner), never a live desktop session.
func requireIsolatedAgentEnvironment(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ssh-agent"); err != nil {
		t.Skip("ssh-agent not on PATH")
	}
	lockRealAgentTests(t)
	procs, err := (Inspector{}).Agents()
	if err != nil {
		t.Skipf("cannot enumerate /proc: %v", err)
	}
	prober := SocketProber{}
	for _, p := range procs {
		if p.Socket != "" && prober.Reachable(p.Socket) {
			t.Skipf("a real ssh-agent (pid %d, socket %s) is already reachable on this machine — "+
				"these five-state integration tests need an isolated PID namespace (e.g. the tier-1 "+
				"container), not a live desktop session", p.PID, p.Socket)
		}
	}
}

func newRealManager() Manager {
	return Manager{
		Prober:    SocketProber{},
		Inspector: Inspector{},
		Runner:    ExecRunner{},
		Signaler:  SysSignaler{},
		Locker:    FlockLocker{Wait: 2 * time.Second},
	}
}

func realCfg(t *testing.T) EnsureConfig {
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
	t.Fatalf("pid %d did not exit in time", pid)
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
	pid, err := (ExecRunner{}).Start(sock)
	if err != nil {
		t.Fatalf("start foreign ssh-agent: %v", err)
	}
	t.Cleanup(func() { stopAgent(t, pid) })
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if (SocketProber{}).Reachable(sock) {
			return pid
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("foreign ssh-agent pid %d never became reachable on %s", pid, sock)
	return pid
}

func TestEnsureAgentRealClean(t *testing.T) {
	requireIsolatedAgentEnvironment(t)
	m := newRealManager()
	cfg := realCfg(t)

	res, err := m.EnsureAgent(cfg, nil)
	if err != nil {
		t.Fatalf("EnsureAgent: %v", err)
	}
	t.Cleanup(func() { stopAgent(t, res.Started) })

	if res.Situation != SituationClean {
		t.Errorf("Situation = %s, want clean", res.Situation)
	}
	if res.Started == 0 {
		t.Error("expected a started pid")
	}
	if !m.Prober.Reachable(cfg.FixedSock) {
		t.Error("fixed socket not reachable after a clean start")
	}
}

func TestEnsureAgentRealHealthyReuse(t *testing.T) {
	requireIsolatedAgentEnvironment(t)
	m := newRealManager()
	cfg := realCfg(t)

	res1, err := m.EnsureAgent(cfg, nil)
	if err != nil {
		t.Fatalf("first EnsureAgent: %v", err)
	}
	t.Cleanup(func() { stopAgent(t, res1.Started) })

	res2, err := m.EnsureAgent(cfg, nil)
	if err != nil {
		t.Fatalf("second EnsureAgent: %v", err)
	}
	if res2.Situation != SituationHealthy {
		t.Errorf("Situation = %s, want healthy", res2.Situation)
	}
	if res2.Started != 0 {
		t.Errorf("expected no new agent started on reuse, got pid %d", res2.Started)
	}
}

// TestEnsureAgentRealReachableButEmptyIsHealthy covers the D1 case: an
// agent with zero keys loaded (ssh-add -l exits 1) must still be treated as
// healthy, never killed and replaced.
func TestEnsureAgentRealReachableButEmptyIsHealthy(t *testing.T) {
	requireIsolatedAgentEnvironment(t)
	m := newRealManager()
	cfg := realCfg(t)

	res1, err := m.EnsureAgent(cfg, nil)
	if err != nil {
		t.Fatalf("first EnsureAgent: %v", err)
	}
	t.Cleanup(func() { stopAgent(t, res1.Started) })

	// No keys were ever added, so the agent's own reply to
	// SSH_AGENTC_REQUEST_IDENTITIES lists zero identities — the real-world
	// equivalent of `ssh-add -l` exiting 1. SocketProber's handshake already
	// exercises exactly that round trip, so a second probe here is redundant;
	// the point being tested is that EnsureAgent still calls this healthy.

	res2, err := m.EnsureAgent(cfg, nil)
	if err != nil {
		t.Fatalf("second EnsureAgent: %v", err)
	}
	if res2.Situation != SituationHealthy {
		t.Errorf("Situation = %s, want healthy (an empty agent is still healthy)", res2.Situation)
	}
	if res2.Started != 0 {
		t.Error("an empty-but-reachable agent must never be replaced")
	}
}

func TestEnsureAgentRealZombie(t *testing.T) {
	requireIsolatedAgentEnvironment(t)
	m := newRealManager()
	cfg := realCfg(t)

	res1, err := m.EnsureAgent(cfg, nil)
	if err != nil {
		t.Fatalf("first EnsureAgent: %v", err)
	}
	killAgentLeavingSocket(t, res1.Started)
	if m.Prober.Reachable(cfg.FixedSock) {
		t.Fatal("socket should be dead after SIGKILL")
	}

	res2, err := m.EnsureAgent(cfg, nil)
	if err != nil {
		t.Fatalf("second EnsureAgent: %v", err)
	}
	t.Cleanup(func() { stopAgent(t, res2.Started) })

	if res2.Situation != SituationZombie {
		t.Errorf("Situation = %s, want zombie", res2.Situation)
	}
	if len(res2.Reaped.Terminated) == 0 && len(res2.Reaped.RemovedSockets) == 0 {
		t.Error("expected the dead agent/socket to be reaped")
	}
	if !m.Prober.Reachable(cfg.FixedSock) {
		t.Error("expected a fresh healthy agent after the zombie reap")
	}
}

// TestEnsureAgentRealForeignAdopted covers state D: a healthy agent sshakku
// did not start must be adopted via the fixed-socket symlink, not killed.
func TestEnsureAgentRealForeignAdopted(t *testing.T) {
	requireIsolatedAgentEnvironment(t)
	m := newRealManager()
	cfg := realCfg(t)

	foreignSock := filepath.Join(shortDir(t), "foreign.sock")
	foreignPID := startForeignAgent(t, foreignSock)

	res, err := m.EnsureAgent(cfg, nil)
	if err != nil {
		t.Fatalf("EnsureAgent: %v", err)
	}

	if res.Situation != SituationForeign {
		t.Errorf("Situation = %s, want foreign", res.Situation)
	}
	if res.Started != 0 {
		t.Error("must never start a competing agent when a healthy foreign one exists")
	}
	if res.Adopted == nil || res.Adopted.PID != foreignPID {
		t.Errorf("Adopted = %+v, want pid %d", res.Adopted, foreignPID)
	}
	if res.Anomaly == "" {
		t.Error("adopting a foreign agent must report an anomaly")
	}
	if !m.Prober.Reachable(cfg.FixedSock) {
		t.Error("fixed socket should reach the adopted foreign agent")
	}
	// The foreign agent itself must still be alive — never killed.
	if err := syscall.Kill(foreignPID, 0); err != nil {
		t.Errorf("foreign agent pid %d was killed, want it left running: %v", foreignPID, err)
	}
}

// TestEnsureAgentRealDisasterReapsAndAdoptsLowestPID covers state E: a dead
// agent of ours plus two healthy foreign agents. EnsureAgent must reap the
// dead one, adopt the lowest-pid healthy foreign one, and report disaster.
func TestEnsureAgentRealDisasterReapsAndAdoptsLowestPID(t *testing.T) {
	requireIsolatedAgentEnvironment(t)
	m := newRealManager()
	cfg := realCfg(t)

	res1, err := m.EnsureAgent(cfg, nil)
	if err != nil {
		t.Fatalf("seed EnsureAgent: %v", err)
	}
	killAgentLeavingSocket(t, res1.Started) // now dead-ours

	sockA := filepath.Join(shortDir(t), "foreign-a.sock")
	sockB := filepath.Join(shortDir(t), "foreign-b.sock")
	pidA := startForeignAgent(t, sockA)
	pidB := startForeignAgent(t, sockB)
	lowest := pidA
	if pidB < pidA {
		lowest = pidB
	}

	res2, err := m.EnsureAgent(cfg, nil)
	if err != nil {
		t.Fatalf("EnsureAgent: %v", err)
	}

	if res2.Situation != SituationDisaster {
		t.Errorf("Situation = %s, want disaster", res2.Situation)
	}
	if len(res2.Reaped.RemovedSockets) == 0 {
		t.Error("expected the dead-ours socket to be reaped")
	}
	if res2.Adopted == nil || res2.Adopted.PID != lowest {
		t.Errorf("Adopted = %+v, want the lowest-pid healthy foreign agent (%d)", res2.Adopted, lowest)
	}
	for _, pid := range []int{pidA, pidB} {
		if err := syscall.Kill(pid, 0); err != nil {
			t.Errorf("foreign agent pid %d was killed, want both left running: %v", pid, err)
		}
	}
}
