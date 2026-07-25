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

// TestSecretServiceUnlockClientError covers Unlock's client-error branch: the
// collection resolves but the D-Bus Unlock call fails.
func TestSecretServiceUnlockClientError(t *testing.T) {
	client := &fakeSecretServiceClient{collection: "/org/collection/sshakku", unlockErr: errors.New("unlock refused")}
	b := &SecretServiceBackend{Client: client, User: "u"}
	if err := b.Unlock(); err == nil {
		t.Fatal("Unlock returned nil, want the client's unlock error")
	}
}

// TestSecretServiceLockCollectionError covers Lock's resolve-failure branch:
// the collection cannot be resolved, so Lock returns before touching the bus.
func TestSecretServiceLockCollectionError(t *testing.T) {
	client := &fakeSecretServiceClient{collectionErr: errors.New("no such collection")}
	b := &SecretServiceBackend{Client: client, User: "u"}
	if err := b.Lock(); err == nil {
		t.Fatal("Lock returned nil, want the collection-resolve error")
	}
	if len(client.locked) != 0 {
		t.Fatalf("Lock must not call the bus when the collection cannot resolve, got %v", client.locked)
	}
}
