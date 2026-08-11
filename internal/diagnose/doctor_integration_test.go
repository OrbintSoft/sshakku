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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// requireIsolatedAgentEnvironment skips unless no ssh-agent is already
// reachable on this machine: agent.Inspector scans the real, machine-wide
// /proc, which a live desktop session (or another test's leftovers) would
// pollute — these tests need an isolated PID namespace (the container test
// suite, or a fresh CI runner), never a live login.
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

// shortDir returns a fresh, auto-cleaned temp directory with a short path.
// Unlike t.TempDir(), which nests the (sub)test name under the OS temp root
// (e.g. macOS's /var/folders/xx/.../T/TestName.../001/), it stays well under
// the 104-byte sun_path limit unix sockets are bound under on BSD/Darwin.
func shortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "sshakku") //nolint:usetesting // t.TempDir() is the long macOS path the comment above is about
	require.NoError(t, err, "mkdir temp")
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
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

	dir := shortDir(t)
	cfg := agent.EnsureConfig{
		FixedSock: filepath.Join(dir, "agent.sock"),
		LegacyDir: filepath.Join(dir, "legacy"),
		StatePath: filepath.Join(dir, "agent.state"),
		LockPath:  filepath.Join(dir, "agent.lock"),
		OurUID:    ourUID(),
	}
	m := realManager()

	res1, err := m.EnsureAgent(cfg, nil)
	require.NoError(t, err, "seed EnsureAgent")

	// EnvSock and the askpass exports mirror a shell the login hook has already
	// wired (the normal case once shell-init and askpass-env have run), so the
	// report's findings reflect the agent's own health — what this test is
	// about — rather than an environment this test never set up.
	in := Inputs{
		FixedSock: cfg.FixedSock, EnvSock: cfg.FixedSock, LegacyDir: cfg.LegacyDir,
		StatePath: cfg.StatePath, OurUID: cfg.OurUID,
		EnvAskpass: "/usr/local/bin/sshakku", EnvAskpassRequire: "force",
	}

	before := Gather(in, agent.Inspector{}, agent.SocketProber{}, nil, nil, nil, nil)
	require.Equal(t, StateOursHealthy, before.State, "the agent this test then crashes must be healthy to start with")

	// Simulate a real crash: SIGKILL, no graceful socket cleanup by ssh-agent
	// itself. Whether or not something in this PID namespace reaps it, the
	// agent.state file EnsureAgent wrote survives, which is what lets doctor
	// detect this precise case even once the process is fully gone from /proc.
	_ = syscall.Kill(res1.Started, syscall.SIGKILL)
	waitDead(t, res1.Started)

	after := Gather(in, agent.Inspector{}, agent.SocketProber{}, nil, nil, nil, nil)
	assert.Equal(t, StateOursZombie, after.State, "an agent of ours that died leaves its state behind")
	assert.Falsef(t, hasFinding(after, "no problems detected"),
		"a report over a crashed agent must not call the machine clean: %v", after.Findings)

	// doctor --fix's actual mechanism (cmd/sshakku's runFix): EnsureAgent,
	// then re-Gather.
	res2, err := m.EnsureAgent(cfg, nil)
	require.NoError(t, err, "fix EnsureAgent")
	t.Cleanup(func() { _ = syscall.Kill(res2.Started, syscall.SIGTERM) })
	assert.Equal(t, agent.SituationZombie, res2.Situation, "the fix must recognise what it is repairing")

	fixed := Gather(in, agent.Inspector{}, agent.SocketProber{}, nil, nil, nil, nil)
	assert.Equal(t, StateOursHealthy, fixed.State, "the fix must leave a healthy agent behind")
	assert.Truef(t, hasFinding(fixed, "no problems detected"),
		"a repaired machine must be reported as clean: %v", fixed.Findings)
}

func ourUID() int {
	// Matches EnsureConfig.OurUID's real-world source (os.Getuid()); kept
	// local so this file doesn't need an "os" import just for one call site.
	return int(syscall.Getuid())
}
