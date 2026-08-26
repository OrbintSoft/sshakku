package agent

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/agent/inspect"

	"github.com/OrbintSoft/sshakku/internal/agent/inspect/inspecttest"

	"github.com/OrbintSoft/sshakku/internal/testtmp"
)

// errNoSshAgent is the failure this test hands its seam, standing for a real one the
// code under test cannot be made to produce on demand.
var errNoSshAgent = errors.New("no ssh-agent")

// TestSituationStringUnknown covers Situation.String's default arm for a value
// outside the defined set.
func TestSituationStringUnknown(t *testing.T) {
	assert.Equal(t, "unknown", Situation(99).String(), "a situation outside the defined set")
}

// TestEnsureAgentReapError covers EnsureAgent's error return when the reap pass
// cannot enumerate processes (a missing procfs root).
func TestEnsureAgentReapError(t *testing.T) {
	dir := testtmp.ShortDir(t)
	fixed := filepath.Join(dir, "agent.sock")
	m := Manager{
		Prober:    mapProber{}, // fixed silent → past the fast path
		Inspector: inspect.Inspector{ProcRoot: filepath.Join(dir, "nope")},
		Runner:    &recordRunner{},
		Signaler:  &recordSignaler{},
	}
	_, err := m.EnsureAgent(t.Context(), EnsureConfig{FixedSock: fixed, OurUID: 1000}, nil)
	assert.Error(t, err, "a reap that cannot read the process list must be reported")
}

// TestManagerReapInspectError covers Reap's error return when the process list
// cannot be read (a missing procfs root).
func TestManagerReapInspectError(t *testing.T) {
	m := Manager{Inspector: inspect.Inspector{ProcRoot: filepath.Join(t.TempDir(), "nope")}}
	_, err := m.Reap(t.Context(), 1000)
	assert.Error(t, err, "a process list that cannot be read must be reported")
}

// TestManagerStartRunnerError covers Start's error return when the runner fails to
// launch an agent — the state file must not be written.
func TestManagerStartRunnerError(t *testing.T) {
	dir := testtmp.ShortDir(t)
	socket := filepath.Join(dir, "agent.sock")
	state := filepath.Join(dir, "agent.state")
	m := Manager{Prober: mapProber{}, Runner: &recordRunner{err: errNoSshAgent}}

	_, err := m.Start(t.Context(), socket, state)
	assert.Error(t, err, "a runner that fails must be reported")
	_, err = os.Stat(state)
	assert.ErrorIs(t, err, os.ErrNotExist, "no state file must be written on a runner failure")
}

// TestManagerStartStateWriteError covers Start's non-fatal path: the agent came up
// but recording its state failed. It must still return the pid alongside the error.
func TestManagerStartStateWriteError(t *testing.T) {
	dir := testtmp.ShortDir(t)
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
	dir := testtmp.ShortDir(t)
	fixed := filepath.Join(dir, "agent.sock")
	makeSocketFile(t, fixed) // orphan socket, no matching proc

	runner := &recordRunner{pid: 7000}
	log := &fakeLogger{}
	m := Manager{
		Prober:    mapProber{},                                      // fixed is silent
		Inspector: inspect.Inspector{ProcRoot: testtmp.ShortDir(t)}, // no processes at all
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
	dir := testtmp.ShortDir(t)
	fixed := filepath.Join(dir, "agent.sock")
	m := Manager{
		Prober:    mapProber{},
		Inspector: inspect.Inspector{ProcRoot: testtmp.ShortDir(t)},
		Runner:    &recordRunner{err: errNoSshAgent},
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
	dir := testtmp.ShortDir(t)
	fixed := filepath.Join(dir, "agent.sock")
	proc := testtmp.ShortDir(t)
	foreignSock := filepath.Join(dir, "foreign.sock")

	makeSocketFile(t, fixed)                                                           // orphan socket at the fixed path
	inspecttest.FakeProc(t, proc, 300, []string{"ssh-agent", "-a", foreignSock}, 1000) // healthy foreign, unrelated socket

	log := &fakeLogger{}
	m := Manager{
		Prober:    mapProber{foreignSock: true}, // fixed silent, foreign healthy
		Inspector: inspect.Inspector{ProcRoot: proc},
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
	dir := testtmp.ShortDir(t)
	fixed := filepath.Join(dir, "no-such-dir", "agent.sock") // parent missing → symlink fails
	proc := testtmp.ShortDir(t)
	foreignSock := filepath.Join(dir, "foreign.sock")
	inspecttest.FakeProc(t, proc, 300, []string{"ssh-agent", "-a", foreignSock}, 1000)

	m := Manager{
		Prober:    mapProber{foreignSock: true},
		Inspector: inspect.Inspector{ProcRoot: proc},
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
	dir := testtmp.ShortDir(t)

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
