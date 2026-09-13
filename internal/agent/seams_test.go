package agent

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/OrbintSoft/sshakku/internal/agent/inspect"
	"github.com/OrbintSoft/sshakku/internal/testtmp"
)

// errProcessScanFailed is the failure this test hands its seam, standing for a real one the
// code under test cannot be made to produce on demand.
var errProcessScanFailed = errors.New("process scan failed")

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
		return nil, errProcessScanFailed
	}
	return l.inner.Agents()
}

// TestEnsureAgentForeignSurveyError covers EnsureAgent's error return when the
// healthy-agent survey (the second process scan, after a successful reap) fails.
//
// What a session must not do is decide there are no agents about because it
// could not look: that reads as an empty machine, and an empty machine is one
// this starts another agent on. The survey and the reap go through the same
// collaborator, which is why the failure is aimed at the second call alone.
//
// The lifecycle this exercises is nobody's platform in particular, so neither
// is the test: every system that runs EnsureAgent runs this branch.
func TestEnsureAgentForeignSurveyError(t *testing.T) {
	dir := testtmp.ShortDir(t)
	fixed := filepath.Join(dir, "agent.sock")
	lister := &nthErrLister{inner: inspect.Inspector{ProcRoot: testtmp.ShortDir(t)}, failOn: 2}
	m := Manager{
		Prober:    mapProber{}, // fixed silent.
		Inspector: lister,
		Runner:    &recordRunner{},
		Signaler:  &recordSignaler{},
	}
	_, err := m.EnsureAgent(t.Context(), EnsureConfig{FixedSock: fixed, OurUID: 1000}, nil)
	assert.Error(t, err, "a healthy-agent survey that cannot read the process list must be reported")
	assert.Equal(t, 2, lister.calls, "Agents is called twice: reap then survey")
}
