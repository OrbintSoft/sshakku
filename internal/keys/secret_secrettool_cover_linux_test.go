//go:build linux

package keys

import (
	"errors"
	"testing"
)

// TestSecretToolStoreNonZeroExit covers Store's non-zero-exit branch: a failing
// secret-tool store is reported as an error carrying its stderr.
func TestSecretToolStoreNonZeroExit(t *testing.T) {
	r := newFakeRunner().on("secret-tool", func(Cmd) (Result, error) {
		return Result{Code: 1, Stderr: []byte("store denied")}, nil
	})
	b := SecretToolBackend{Runner: r, User: "u"}
	if err := b.Store("svc", "label", "pass"); err == nil {
		t.Fatal("Store returned nil, want an error on a non-zero secret-tool exit")
	}
}

// TestSecretToolStoreRunError covers Store's start-failure branch: secret-tool
// cannot be run at all, which propagates as an error.
func TestSecretToolStoreRunError(t *testing.T) {
	boom := errors.New("secret-tool exec boom")
	r := newFakeRunner().on("secret-tool", fails(boom))
	b := SecretToolBackend{Runner: r, User: "u"}
	if err := b.Store("svc", "label", "pass"); !errors.Is(err, boom) {
		t.Fatalf("error = %v, want %v", err, boom)
	}
}
