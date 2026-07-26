//go:build unix

package agent

import (
	"path/filepath"
	"testing"
	"time"
)

func TestFlockLockerSerialises(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.lock")

	held, err := FlockLocker{}.Lock(path)
	if err != nil {
		t.Fatalf("first Lock: %v", err)
	}

	// A second acquirer must wait out its bounded deadline rather than grab the
	// lock while it is held, then proceed unlocked.
	start := time.Now()
	contended, err := FlockLocker{Wait: 120 * time.Millisecond, Poll: 20 * time.Millisecond}.Lock(path)
	if err != nil {
		t.Fatalf("contended Lock: %v", err)
	}
	if waited := time.Since(start); waited < 100*time.Millisecond {
		t.Errorf("contended Lock returned after %v, want it to wait out the deadline", waited)
	}
	contended()
	held()

	// Once free, acquiring takes the lock again without exhausting the deadline.
	free, err := FlockLocker{Wait: time.Second, Poll: 20 * time.Millisecond}.Lock(path)
	if err != nil {
		t.Fatalf("Lock after release: %v", err)
	}
	free()
}

func TestFlockLockerCreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.lock")
	unlock, err := FlockLocker{}.Lock(path)
	if err != nil {
		t.Fatalf("Lock on a missing path should create it: %v", err)
	}
	unlock()
}

func TestFlockLockerOpenError(t *testing.T) {
	// The lock file's parent directory does not exist, so opening it fails before
	// any flock attempt.
	path := filepath.Join(t.TempDir(), "no-such-dir", "agent.lock")
	if _, err := (FlockLocker{}).Lock(path); err == nil {
		t.Fatal("want an error when the lock file cannot be opened")
	}
}
