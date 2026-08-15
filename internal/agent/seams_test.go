//go:build unix

package agent

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"

	"github.com/OrbintSoft/sshakku/internal/agent/inspect"
)

// nthErrLister wraps a ProcLister and forces the failOn-th (1-based) Agents call to
// error, so EnsureAgent's reap and healthy-survey scans can be made to succeed and
// fail independently even though both go through the same collaborator.
type nthErrLister struct {
	inner  ProcLister
	failOn int
	calls  int
}

func (l *nthErrLister) Agents() ([]inspect.AgentProc, error) {
	l.calls++
	if l.calls == l.failOn {
		return nil, errors.New("process scan failed")
	}
	return l.inner.Agents()
}

// TestEnsureAgentForeignSurveyError covers EnsureAgent's error return when the
// healthy-agent survey (the second process scan, after a successful reap) fails.
func TestEnsureAgentForeignSurveyError(t *testing.T) {
	dir := shortDir(t)
	fixed := filepath.Join(dir, "agent.sock")
	lister := &nthErrLister{inner: inspect.Inspector{ProcRoot: shortDir(t)}, failOn: 2}
	m := Manager{
		Prober:    mapProber{}, // fixed silent
		Inspector: lister,
		Runner:    &recordRunner{},
		Signaler:  &recordSignaler{},
	}
	_, err := m.EnsureAgent(t.Context(), EnsureConfig{FixedSock: fixed, OurUID: 1000}, nil)
	assert.Error(t, err, "a healthy-agent survey that cannot read the process list must be reported")
	assert.Equal(t, 2, lister.calls, "Agents is called twice: reap then survey")
}

// TestFlockLockerFlockError covers Lock's fatal branch: a flock failure other than
// EWOULDBLOCK closes the file and returns the error rather than proceeding unlocked.
func TestFlockLockerFlockError(t *testing.T) {
	orig := flock
	t.Cleanup(func() { flock = orig })
	flock = func(int, int) error { return unix.EINVAL }

	path := filepath.Join(t.TempDir(), "agent.lock")
	_, err := (FlockLocker{}).Lock(path)
	assert.Error(t, err, "a non-EWOULDBLOCK flock failure must be reported")
}
