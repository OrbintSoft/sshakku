package agent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeLogger records the level-tagged lines EnsureAgent emits.
type fakeLogger struct{ lines []string }

func (f *fakeLogger) Log(level, message string) error {
	f.lines = append(f.lines, level+" "+message)
	return nil
}

func (f *fakeLogger) hasLevel(level string) bool {
	for _, l := range f.lines {
		if strings.HasPrefix(l, level+" ") {
			return true
		}
	}
	return false
}

// fakeLocker records the lock path and release calls. onLock, if set, runs while
// the lock is held — before the under-lock re-check — so a test can make the fixed
// socket appear healthy at that moment, as a concurrent login would.
type fakeLocker struct {
	locked   []string
	unlocked int
	err      error
	onLock   func()
}

func (f *fakeLocker) Lock(path string) (func(), error) {
	f.locked = append(f.locked, path)
	if f.err != nil {
		return nil, f.err
	}
	if f.onLock != nil {
		f.onLock()
	}
	return func() { f.unlocked++ }, nil
}

func TestEnsureAgentHealthy(t *testing.T) {
	dir := shortDir(t)
	fixed := filepath.Join(dir, "agent.sock")
	runner := &recordRunner{pid: 1}

	m := Manager{Prober: mapProber{fixed: true}, Runner: runner, Signaler: &recordSignaler{}}
	res, err := m.EnsureAgent(t.Context(), EnsureConfig{FixedSock: fixed, StatePath: filepath.Join(dir, "st"), OurUID: 1000}, nil)
	require.NoError(t, err)
	assert.Equal(t, SituationHealthy, res.Situation, "situation")
	assert.Equal(t, fixed, res.LiveSock, "the live socket is the fixed one")
	assert.Empty(t, runner.started, "the healthy path must not start an agent")
}

func TestEnsureAgentClean(t *testing.T) {
	dir := shortDir(t)
	fixed := filepath.Join(dir, "agent.sock")
	state := filepath.Join(dir, "agent.state")
	runner := &recordRunner{pid: 4242}

	m := Manager{
		Prober:    mapProber{}, // nothing reachable
		Inspector: Inspector{ProcRoot: shortDir(t)},
		Runner:    runner,
		Signaler:  &recordSignaler{},
	}
	res, err := m.EnsureAgent(t.Context(), EnsureConfig{FixedSock: fixed, StatePath: state, OurUID: 1000}, nil)
	require.NoError(t, err)
	assert.Equal(t, SituationClean, res.Situation, "situation")
	assert.Equal(t, fixed, res.LiveSock, "the live socket is the fixed one")
	assert.Equal(t, 4242, res.Started, "the pid started")
	assert.Equal(t, fixed, runner.started, "the socket the agent was started on")

	st, err := ReadState(state)
	require.NoError(t, err, "ReadState")
	assert.Equal(t, 4242, st.PID, "the state records the started pid")
}

func TestEnsureAgentZombie(t *testing.T) {
	dir := shortDir(t)
	fixed := filepath.Join(dir, "agent.sock")
	state := filepath.Join(dir, "agent.state")
	proc := shortDir(t)

	makeSocketFile(t, fixed)                                         // a real stale socket at our path
	fakeProc(t, proc, 200, []string{"ssh-agent", "-a", fixed}, 1000) // dead agent of ours

	runner := &recordRunner{pid: 7000}
	sig := &recordSignaler{}
	m := Manager{Prober: mapProber{}, Inspector: Inspector{ProcRoot: proc}, Runner: runner, Signaler: sig}
	log := &fakeLogger{}

	res, err := m.EnsureAgent(t.Context(), EnsureConfig{FixedSock: fixed, StatePath: state, OurUID: 1000}, log)
	require.NoError(t, err)
	assert.Equal(t, SituationZombie, res.Situation, "situation")
	assert.Contains(t, sig.killed, 200, "the dead agent of ours must be reaped")
	assert.Equal(t, fixed, runner.started, "ours must be restarted on the fixed socket")
	assert.True(t, log.hasLevel("INFO"), "the reap and restart must be logged at INFO")
}

func TestEnsureAgentForeign(t *testing.T) {
	dir := shortDir(t)
	fixed := filepath.Join(dir, "agent.sock")
	proc := shortDir(t)
	foreignSock := filepath.Join(dir, "foreign.sock")

	fakeProc(t, proc, 300, []string{"ssh-agent", "-a", foreignSock}, 1000)

	runner := &recordRunner{pid: 1}
	m := Manager{
		Prober:    mapProber{foreignSock: true}, // fixed silent, foreign healthy
		Inspector: Inspector{ProcRoot: proc},
		Runner:    runner,
		Signaler:  &recordSignaler{},
	}
	log := &fakeLogger{}

	cfg := EnsureConfig{FixedSock: fixed, LegacyDir: "/nope", StatePath: filepath.Join(dir, "st"), OurUID: 1000}
	res, err := m.EnsureAgent(t.Context(), cfg, log)
	require.NoError(t, err)
	assert.Equal(t, SituationForeign, res.Situation, "situation")
	require.NotNil(t, res.Adopted, "an agent must have been adopted")
	assert.Equal(t, 300, res.Adopted.PID, "the adopted pid")
	assert.NotEmpty(t, res.Anomaly, "foreign adoption must report an anomaly")
	assert.True(t, log.hasLevel("WARN"), "the anomaly must be logged at WARN")
	assert.Empty(t, runner.started, "adoption must not start a new agent")
	assert.Equal(t, fixed, res.LiveSock, "the live socket is still the fixed one")

	target, err := os.Readlink(fixed)
	require.NoError(t, err, "readlink(fixed)")
	assert.Equal(t, foreignSock, target, "the fixed path must point at the adopted socket")
}

func TestEnsureAgentDisasterMultiple(t *testing.T) {
	dir := shortDir(t)
	fixed := filepath.Join(dir, "agent.sock")
	proc := shortDir(t)
	f1 := filepath.Join(dir, "f1.sock")
	f2 := filepath.Join(dir, "f2.sock")

	fakeProc(t, proc, 400, []string{"ssh-agent", "-a", f2}, 1000)
	fakeProc(t, proc, 300, []string{"ssh-agent", "-a", f1}, 1000)

	m := Manager{
		Prober:    mapProber{f1: true, f2: true},
		Inspector: Inspector{ProcRoot: proc},
		Runner:    &recordRunner{},
		Signaler:  &recordSignaler{},
	}
	res, err := m.EnsureAgent(t.Context(), EnsureConfig{FixedSock: fixed, OurUID: 1000}, nil)
	require.NoError(t, err)
	assert.Equal(t, SituationDisaster, res.Situation, "situation")
	require.NotNil(t, res.Adopted, "an agent must have been adopted")
	assert.Equal(t, 300, res.Adopted.PID, "the lowest pid is adopted")
	assert.Contains(t, res.Anomaly, "2 healthy agents", "the anomaly must note how many were found")

	target, _ := os.Readlink(fixed)
	assert.Equal(t, f1, target, "the fixed path must point at the lowest pid's socket")
}

func TestEnsureAgentDisasterReapAndAdopt(t *testing.T) {
	dir := shortDir(t)
	fixed := filepath.Join(dir, "agent.sock")
	proc := shortDir(t)
	foreignSock := filepath.Join(dir, "foreign.sock")

	makeSocketFile(t, fixed)                                               // stale socket of ours
	fakeProc(t, proc, 200, []string{"ssh-agent", "-a", fixed}, 1000)       // dead ours
	fakeProc(t, proc, 300, []string{"ssh-agent", "-a", foreignSock}, 1000) // healthy foreign

	sig := &recordSignaler{}
	m := Manager{
		Prober:    mapProber{foreignSock: true},
		Inspector: Inspector{ProcRoot: proc},
		Runner:    &recordRunner{},
		Signaler:  sig,
	}
	res, err := m.EnsureAgent(t.Context(), EnsureConfig{FixedSock: fixed, OurUID: 1000}, nil)
	require.NoError(t, err)
	assert.Equal(t, SituationDisaster, res.Situation, "situation")
	assert.Contains(t, sig.killed, 200, "the dead agent of ours must be reaped")
	require.NotNil(t, res.Adopted, "an agent must have been adopted")
	assert.Equal(t, 300, res.Adopted.PID, "the healthy foreign agent is adopted")

	target, _ := os.Readlink(fixed)
	assert.Equal(t, foreignSock, target, "the fixed path must point at the adopted socket")
}

func TestClearStalePath(t *testing.T) {
	dir := shortDir(t)

	sock := filepath.Join(dir, "a.sock")
	makeSocketFile(t, sock)
	assert.True(t, clearStalePath(sock), "a socket must be cleared")
	_, err := os.Lstat(sock)
	assert.ErrorIs(t, err, os.ErrNotExist, "the socket must be gone")

	link := filepath.Join(dir, "link")
	require.NoError(t, os.Symlink("/dangling", link))
	assert.True(t, clearStalePath(link), "a symlink must be cleared")
	_, err = os.Lstat(link)
	assert.ErrorIs(t, err, os.ErrNotExist, "the symlink must be gone")

	reg := filepath.Join(dir, "regular")
	require.NoError(t, os.WriteFile(reg, []byte("x"), 0o600))
	assert.False(t, clearStalePath(reg), "a regular file must not be cleared")
	_, err = os.Lstat(reg)
	assert.NoError(t, err, "the regular file must survive")

	assert.False(t, clearStalePath(filepath.Join(dir, "missing")), "a missing path reports false")
}

func TestEnsureAgentFastPathSkipsLock(t *testing.T) {
	dir := shortDir(t)
	fixed := filepath.Join(dir, "agent.sock")
	lk := &fakeLocker{}

	m := Manager{Prober: mapProber{fixed: true}, Runner: &recordRunner{}, Signaler: &recordSignaler{}, Locker: lk}
	res, err := m.EnsureAgent(t.Context(), EnsureConfig{FixedSock: fixed, LockPath: filepath.Join(dir, "lock"), OurUID: 1000}, nil)
	require.NoError(t, err)
	assert.Equal(t, SituationHealthy, res.Situation, "situation")
	assert.Empty(t, lk.locked, "the healthy fast path must not lock")
}

func TestEnsureAgentLocksMutatePath(t *testing.T) {
	dir := shortDir(t)
	fixed := filepath.Join(dir, "agent.sock")
	lock := filepath.Join(dir, "agent.lock")
	runner := &recordRunner{pid: 4242}
	lk := &fakeLocker{}

	m := Manager{
		Prober:    mapProber{}, // silent
		Inspector: Inspector{ProcRoot: shortDir(t)},
		Runner:    runner,
		Signaler:  &recordSignaler{},
		Locker:    lk,
	}
	res, err := m.EnsureAgent(t.Context(), EnsureConfig{FixedSock: fixed, StatePath: filepath.Join(dir, "st"), LockPath: lock, OurUID: 1000}, nil)
	require.NoError(t, err)
	assert.Equal(t, SituationClean, res.Situation, "situation")
	assert.Equal(t, fixed, runner.started, "the socket the agent was started on")
	assert.Equal(t, []string{lock}, lk.locked, "a single lock, on the configured path")
	assert.Equal(t, 1, lk.unlocked, "released exactly once, by the deferred release")
}

func TestEnsureAgentDoubleCheckUnderLock(t *testing.T) {
	dir := shortDir(t)
	fixed := filepath.Join(dir, "agent.sock")
	runner := &recordRunner{pid: 1}
	sig := &recordSignaler{}
	prober := mapProber{} // silent on the first check

	// A concurrent login starts ours while we hold the lock: the under-lock
	// re-check must then find it healthy and neither reap nor start.
	lk := &fakeLocker{onLock: func() { prober[fixed] = true }}
	m := Manager{Prober: prober, Inspector: Inspector{ProcRoot: shortDir(t)}, Runner: runner, Signaler: sig, Locker: lk}

	res, err := m.EnsureAgent(t.Context(), EnsureConfig{FixedSock: fixed, StatePath: filepath.Join(dir, "st"), LockPath: filepath.Join(dir, "lock"), OurUID: 1000}, nil)
	require.NoError(t, err)
	assert.Equal(t, SituationHealthy, res.Situation, "the under-lock re-check must find ours healthy")
	assert.Equal(t, fixed, res.LiveSock, "the live socket is the fixed one")
	assert.Empty(t, runner.started, "the re-check found ours healthy, so nothing must be started")
	assert.Empty(t, sig.killed, "the re-check found ours healthy, so nothing must be reaped")
	assert.Equal(t, 1, lk.unlocked, "the lock must be released even on the healthy re-check")
}

func TestEnsureAgentLockError(t *testing.T) {
	dir := shortDir(t)
	fixed := filepath.Join(dir, "agent.sock")
	runner := &recordRunner{pid: 1}
	lk := &fakeLocker{err: errors.New("cannot open lock")}

	m := Manager{Prober: mapProber{}, Inspector: Inspector{ProcRoot: shortDir(t)}, Runner: runner, Signaler: &recordSignaler{}, Locker: lk}
	_, err := m.EnsureAgent(t.Context(), EnsureConfig{FixedSock: fixed, LockPath: filepath.Join(dir, "lock"), OurUID: 1000}, nil)
	assert.Error(t, err, "a lock that cannot be acquired must be reported")
	assert.Empty(t, runner.started, "nothing must be started after a lock failure")
}

func TestSituationString(t *testing.T) {
	for s, want := range map[Situation]string{
		SituationHealthy:  "healthy",
		SituationClean:    "clean",
		SituationZombie:   "zombie",
		SituationForeign:  "foreign",
		SituationDisaster: "disaster",
	} {
		assert.Equalf(t, want, s.String(), "Situation(%d).String()", s)
	}
}
