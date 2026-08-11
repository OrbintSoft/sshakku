//go:build unix

package agent

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlockLockerSerialises(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.lock")

	held, err := FlockLocker{}.Lock(path)
	require.NoError(t, err, "first Lock")

	// A second acquirer must wait out its bounded deadline rather than grab the
	// lock while it is held, then proceed unlocked.
	start := time.Now()
	contended, err := FlockLocker{Wait: 120 * time.Millisecond, Poll: 20 * time.Millisecond}.Lock(path)
	require.NoError(t, err, "contended Lock")
	assert.GreaterOrEqual(t, time.Since(start), 100*time.Millisecond,
		"a contended Lock must wait out its deadline rather than take a held lock")
	contended()
	held()

	// Once free, acquiring takes the lock again without exhausting the deadline.
	free, err := FlockLocker{Wait: time.Second, Poll: 20 * time.Millisecond}.Lock(path)
	require.NoError(t, err, "Lock after release")
	free()
}

func TestFlockLockerCreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.lock")
	unlock, err := FlockLocker{}.Lock(path)
	require.NoError(t, err, "Lock on a missing path must create it")
	unlock()
}

func TestFlockLockerOpenError(t *testing.T) {
	// The lock file's parent directory does not exist, so opening it fails before
	// any flock attempt.
	path := filepath.Join(t.TempDir(), "no-such-dir", "agent.lock")
	_, err := (FlockLocker{}).Lock(path)
	assert.Error(t, err, "a lock file that cannot be opened must be reported")
}
