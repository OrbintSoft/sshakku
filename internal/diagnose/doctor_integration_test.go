//go:build unix

package diagnose

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/OrbintSoft/sshakku/internal/agent"
	"golang.org/x/sys/unix"
)

// lockRealAgentTests serialises every real-ssh-agent-spawning test across
// this package AND internal/agent (which has its own copy of this function)
// via a flock on a well-known path. `go test ./...` runs different packages'
// test binaries concurrently by default, and these tests kill processes by
// raw pid — without this lock, one package's cleanup can race pid reuse
// against another package's freshly started ssh-agent and kill the wrong
// process (observed in practice as "start ssh-agent: signal: terminated").
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

// requireIsolatedAgentEnvironment skips unless no ssh-agent is already
// reachable on this machine: agent.Inspector scans the real, machine-wide
// /proc, which a live desktop session (or another test's leftovers) would
// pollute — these tests need an isolated PID namespace (the tier-1
// container, or a fresh CI runner), never a live login.
func requireIsolatedAgentEnvironment(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ssh-agent"); err != nil {
		t.Skip("ssh-agent not on PATH")
	}
	lockRealAgentTests(t)
	procs, err := (agent.Inspector{}).Agents()
	if err != nil {
		t.Skipf("cannot enumerate /proc: %v", err)
	}
	prober := agent.SocketProber{}
	for _, p := range procs {
		if p.Socket != "" && prober.Reachable(p.Socket) {
			t.Skipf("a real ssh-agent (pid %d, socket %s) is already reachable on this machine — "+
				"these integration tests need an isolated PID namespace, not a live desktop session", p.PID, p.Socket)
		}
	}
}

func realManager() agent.Manager {
	return agent.Manager{
		Prober:    agent.SocketProber{},
		Inspector: agent.Inspector{},
		Runner:    agent.ExecRunner{},
		Signaler:  agent.SysSignaler{},
		Locker:    agent.FlockLocker{Wait: 2 * time.Second},
	}
}

func waitDead(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); err != nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	// A zombie with no reaper still answers kill(pid, 0); either way it holds
	// no socket and can't interfere with what this test checks next.
}

// TestDoctorDetectsAndFixesDeadOursAgent drives the real internal/agent
// Manager and internal/diagnose.Gather together: sshakku doctor detecting a
// crashed agent of ours (state C), and doctor --fix — which is exactly
// EnsureAgent run again followed by a re-report — actually resolving it back
// to state B. This is the "doctor rileva i problemi" / "doctor --fix riesce
// a fixare" case for a dead ssh-agent.
func TestDoctorDetectsAndFixesDeadOursAgent(t *testing.T) {
	requireIsolatedAgentEnvironment(t)

	dir := t.TempDir()
	cfg := agent.EnsureConfig{
		FixedSock: filepath.Join(dir, "agent.sock"),
		LegacyDir: filepath.Join(dir, "legacy"),
		StatePath: filepath.Join(dir, "agent.state"),
		LockPath:  filepath.Join(dir, "agent.lock"),
		OurUID:    ourUID(),
	}
	m := realManager()

	res1, err := m.EnsureAgent(cfg, nil)
	if err != nil {
		t.Fatalf("seed EnsureAgent: %v", err)
	}

	// EnvSock mirrors a shell whose SSH_AUTH_SOCK already points at the fixed
	// path (the normal case once shell-init has run), so the report's
	// findings reflect the agent's own health rather than an env mismatch.
	in := Inputs{FixedSock: cfg.FixedSock, EnvSock: cfg.FixedSock, LegacyDir: cfg.LegacyDir, StatePath: cfg.StatePath, OurUID: cfg.OurUID}

	before := Gather(in, agent.Inspector{}, agent.SocketProber{}, nil, nil, nil, nil)
	if before.State != StateOursHealthy {
		t.Fatalf("before crash: State = %v, want ours-healthy", before.State)
	}

	// Simulate a real crash: SIGKILL, no graceful socket cleanup by ssh-agent
	// itself. Whether or not something in this PID namespace reaps it, the
	// agent.state file EnsureAgent wrote survives, which is what lets doctor
	// detect this precise case even once the process is fully gone from /proc.
	_ = syscall.Kill(res1.Started, syscall.SIGKILL)
	waitDead(t, res1.Started)

	after := Gather(in, agent.Inspector{}, agent.SocketProber{}, nil, nil, nil, nil)
	if after.State != StateOursZombie {
		t.Errorf("after crash: State = %v, want ours-zombie", after.State)
	}
	if hasFinding(after, "no problems detected") {
		t.Errorf("after crash: findings claim no problems, got %v", after.Findings)
	}

	// doctor --fix's actual mechanism (cmd/sshakku's runFix): EnsureAgent,
	// then re-Gather.
	res2, err := m.EnsureAgent(cfg, nil)
	if err != nil {
		t.Fatalf("fix EnsureAgent: %v", err)
	}
	t.Cleanup(func() { _ = syscall.Kill(res2.Started, syscall.SIGTERM) })
	if res2.Situation != agent.SituationZombie {
		t.Errorf("fix Situation = %s, want zombie", res2.Situation)
	}

	fixed := Gather(in, agent.Inspector{}, agent.SocketProber{}, nil, nil, nil, nil)
	if fixed.State != StateOursHealthy {
		t.Errorf("after fix: State = %v, want ours-healthy", fixed.State)
	}
	if !hasFinding(fixed, "no problems detected") {
		t.Errorf("after fix: expected a clean report, got %v", fixed.Findings)
	}
}

func ourUID() int {
	// Matches EnsureConfig.OurUID's real-world source (os.Getuid()); kept
	// local so this file doesn't need an "os" import just for one call site.
	return int(syscall.Getuid())
}
