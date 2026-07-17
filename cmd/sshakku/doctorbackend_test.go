package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/keys"
)

// fakeLogger discards every line; tests only care about probeSecretBackend's
// stdout output and return code.
type fakeLogger struct{}

func (fakeLogger) Log(string, string) error { return nil }

// fakeProbeBackend is a keys.SecretBackend whose Store/Lookup/Delete/Unlock/
// Lock behaviour is configured per test, letting probeSecretBackend's
// pass/fail logic be exercised without a real secret store.
type fakeProbeBackend struct {
	storeErr  error
	lookupVal string
	lookupOK  bool
	lookupErr error
	deleteErr error
	unlockErr error
	lockCalls int
}

func (f *fakeProbeBackend) Lookup(string) (string, bool, error) {
	return f.lookupVal, f.lookupOK, f.lookupErr
}
func (f *fakeProbeBackend) Store(string, string, string) error { return f.storeErr }
func (f *fakeProbeBackend) Delete(string) error                { return f.deleteErr }
func (f *fakeProbeBackend) List() ([]string, error)            { return nil, nil }

// fakeProbeSession wraps fakeProbeBackend to also implement keys.SecretSession.
type fakeProbeSession struct{ *fakeProbeBackend }

func (f fakeProbeSession) Unlock() error { return f.unlockErr }
func (f fakeProbeSession) Lock() error   { f.lockCalls++; return nil }

func TestProbeSecretBackendPass(t *testing.T) {
	backend := &fakeProbeBackend{lookupVal: "probe-value", lookupOK: true}
	var buf bytes.Buffer

	got := probeSecretBackend(&buf, fakeLogger{}, backend, "probe-value")
	if got != 0 {
		t.Fatalf("probeSecretBackend = %d, want 0 (pass)", got)
	}
	out := buf.String()
	for _, want := range []string{"store: ok", "lookup: ok", "delete: ok", "backend test: PASS"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestProbeSecretBackendStoreFails(t *testing.T) {
	backend := &fakeProbeBackend{storeErr: errors.New("boom")}
	var buf bytes.Buffer

	got := probeSecretBackend(&buf, fakeLogger{}, backend, "probe-value")
	if got != 1 {
		t.Fatalf("probeSecretBackend = %d, want 1 (fail)", got)
	}
	out := buf.String()
	if !strings.Contains(out, "store: FAILED") || !strings.Contains(out, "delete: ok") || !strings.Contains(out, "backend test: FAIL") {
		t.Errorf("output = %q, want a store failure, an attempted delete, and an overall FAIL", out)
	}
	if strings.Contains(out, "lookup:") {
		t.Errorf("output = %q, want lookup skipped after a store failure", out)
	}
}

func TestProbeSecretBackendLookupMismatch(t *testing.T) {
	backend := &fakeProbeBackend{lookupVal: "wrong-value", lookupOK: true}
	var buf bytes.Buffer

	got := probeSecretBackend(&buf, fakeLogger{}, backend, "probe-value")
	if got != 1 {
		t.Fatalf("probeSecretBackend = %d, want 1 (fail)", got)
	}
	out := buf.String()
	if !strings.Contains(out, "lookup: FAILED") || !strings.Contains(out, "does not match") {
		t.Errorf("output = %q, want a value-mismatch failure", out)
	}
	if !strings.Contains(out, "delete: ok") {
		t.Errorf("output = %q, want delete still attempted after a lookup mismatch", out)
	}
}

func TestProbeSecretBackendLookupMiss(t *testing.T) {
	backend := &fakeProbeBackend{lookupOK: false}
	var buf bytes.Buffer

	got := probeSecretBackend(&buf, fakeLogger{}, backend, "probe-value")
	if got != 1 {
		t.Fatalf("probeSecretBackend = %d, want 1 (fail)", got)
	}
	if !strings.Contains(buf.String(), "not found after storing it") {
		t.Errorf("output = %q, want a not-found failure", buf.String())
	}
}

func TestProbeSecretBackendDeleteFails(t *testing.T) {
	backend := &fakeProbeBackend{lookupVal: "probe-value", lookupOK: true, deleteErr: errors.New("boom")}
	var buf bytes.Buffer

	got := probeSecretBackend(&buf, fakeLogger{}, backend, "probe-value")
	if got != 1 {
		t.Fatalf("probeSecretBackend = %d, want 1 (fail)", got)
	}
	if !strings.Contains(buf.String(), "delete: FAILED") {
		t.Errorf("output = %q, want a delete failure", buf.String())
	}
}

func TestProbeSecretBackendUnlockFails(t *testing.T) {
	backend := &fakeProbeBackend{unlockErr: errors.New("boom")}
	session := fakeProbeSession{backend}
	var buf bytes.Buffer

	got := probeSecretBackend(&buf, fakeLogger{}, session, "probe-value")
	if got != 1 {
		t.Fatalf("probeSecretBackend = %d, want 1 (fail)", got)
	}
	out := buf.String()
	if !strings.Contains(out, "unlock: FAILED") {
		t.Errorf("output = %q, want an unlock failure", out)
	}
	if strings.Contains(out, "store:") {
		t.Errorf("output = %q, want store skipped after an unlock failure", out)
	}
}

func TestProbeSecretBackendUnlocksAndLocks(t *testing.T) {
	backend := &fakeProbeBackend{lookupVal: "probe-value", lookupOK: true}
	session := fakeProbeSession{backend}
	var buf bytes.Buffer

	if got := probeSecretBackend(&buf, fakeLogger{}, session, "probe-value"); got != 0 {
		t.Fatalf("probeSecretBackend = %d, want 0 (pass)", got)
	}
	if !strings.Contains(buf.String(), "unlock: ok") {
		t.Errorf("output = %q, want an unlock: ok line", buf.String())
	}
	if backend.lockCalls != 1 {
		t.Errorf("lockCalls = %d, want 1 (Lock deferred after a successful Unlock)", backend.lockCalls)
	}
}

// keysSecretBackendAssertion pins probeSecretBackend's parameter type against
// keys.SecretBackend so a future interface change here is caught at compile
// time, not by a silent test skip.
var _ keys.SecretBackend = (*fakeProbeBackend)(nil)
