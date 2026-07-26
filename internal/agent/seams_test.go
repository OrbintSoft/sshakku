//go:build unix

package agent

import (
	"errors"
	"net"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// nthErrLister wraps a ProcLister and forces the failOn-th (1-based) Agents call to
// error, so EnsureAgent's reap and healthy-survey scans can be made to succeed and
// fail independently even though both go through the same collaborator.
type nthErrLister struct {
	inner  ProcLister
	failOn int
	calls  int
}

func (l *nthErrLister) Agents() ([]AgentProc, error) {
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
	lister := &nthErrLister{inner: Inspector{ProcRoot: shortDir(t)}, failOn: 2}
	m := Manager{
		Prober:    mapProber{}, // fixed silent
		Inspector: lister,
		Runner:    &recordRunner{},
		Signaler:  &recordSignaler{},
	}
	if _, err := m.EnsureAgent(EnsureConfig{FixedSock: fixed, OurUID: 1000}, nil); err == nil {
		t.Fatal("want an error when the healthy-agent survey cannot read the process list")
	}
	if lister.calls != 2 {
		t.Errorf("Agents called %d times, want 2 (reap then survey)", lister.calls)
	}
}

// TestFlockLockerFlockError covers Lock's fatal branch: a flock failure other than
// EWOULDBLOCK closes the file and returns the error rather than proceeding unlocked.
func TestFlockLockerFlockError(t *testing.T) {
	orig := flock
	t.Cleanup(func() { flock = orig })
	flock = func(int, int) error { return unix.EINVAL }

	path := filepath.Join(t.TempDir(), "agent.lock")
	if _, err := (FlockLocker{}).Lock(path); err == nil {
		t.Fatal("want an error on a non-EWOULDBLOCK flock failure")
	}
}

// TestSocketProberSetDeadlineError covers Reachable's bail-out when the dialed
// connection's deadline cannot be set. The dial itself succeeds against a real
// in-process agent, so only the seamed deadline call fails.
func TestSocketProberSetDeadlineError(t *testing.T) {
	orig := setDeadline
	t.Cleanup(func() { setDeadline = orig })
	setDeadline = func(net.Conn, time.Time) error { return errors.New("cannot set deadline") }

	sock := fakeAgent(t, replyIdentities(1))
	if (SocketProber{Timeout: time.Second}).Reachable(sock) {
		t.Fatal("want unreachable when the connection deadline cannot be set")
	}
}
