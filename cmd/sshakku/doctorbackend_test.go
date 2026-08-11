package main

import (
	"bytes"
	"errors"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/keys"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	lockErr   error
	listVal   []string
	listErr   error
	lockCalls int
}

func (f *fakeProbeBackend) Lookup(string) (string, bool, error) {
	return f.lookupVal, f.lookupOK, f.lookupErr
}
func (f *fakeProbeBackend) Store(string, string, string) error { return f.storeErr }
func (f *fakeProbeBackend) Delete(string) error                { return f.deleteErr }
func (f *fakeProbeBackend) List() ([]string, error)            { return f.listVal, f.listErr }

// fakeProbeSession wraps fakeProbeBackend to also implement keys.SecretSession.
type fakeProbeSession struct{ *fakeProbeBackend }

func (f fakeProbeSession) Unlock() error { return f.unlockErr }
func (f fakeProbeSession) Lock() error   { f.lockCalls++; return f.lockErr }

func TestProbeSecretBackendPass(t *testing.T) {
	backend := &fakeProbeBackend{lookupVal: "probe-value", lookupOK: true}
	var buf bytes.Buffer

	require.Zero(t, probeSecretBackend(&buf, fakeLogger{}, backend, "probe-value"),
		"a wallet that stores, reads back and deletes has passed")
	// Each step is reported on its own, so a reader can see which one a
	// failure would have been.
	for _, want := range []string{"store: ok", "lookup: ok", "delete: ok", "backend test: PASS"} {
		assert.Containsf(t, buf.String(), want, "the report must say %q", want)
	}
}

func TestProbeSecretBackendStoreFails(t *testing.T) {
	backend := &fakeProbeBackend{storeErr: errors.New("boom")}
	var buf bytes.Buffer

	assert.Equal(t, 1, probeSecretBackend(&buf, fakeLogger{}, backend, "probe-value"),
		"a wallet that cannot store has failed")
	out := buf.String()
	assert.Contains(t, out, "store: FAILED", "the step that failed must be named")
	assert.Contains(t, out, "delete: ok", "and the probe entry cleared away regardless")
	assert.Contains(t, out, "backend test: FAIL", "and the verdict must be plain")
	assert.NotContains(t, out, "lookup:",
		"reading back something that was never stored would report a second failure about the first")
}

func TestProbeSecretBackendLookupMismatch(t *testing.T) {
	backend := &fakeProbeBackend{lookupVal: "wrong-value", lookupOK: true}
	var buf bytes.Buffer

	assert.Equal(t, 1, probeSecretBackend(&buf, fakeLogger{}, backend, "probe-value"),
		"a wallet that reads back something else has failed")
	out := buf.String()
	assert.Contains(t, out, "lookup: FAILED", "the step that failed must be named")
	assert.Contains(t, out, "does not match", "and it must say the value came back different, not missing")
	assert.Contains(t, out, "delete: ok", "the probe entry is cleared away regardless")
}

func TestProbeSecretBackendLookupMiss(t *testing.T) {
	backend := &fakeProbeBackend{lookupOK: false}
	var buf bytes.Buffer

	assert.Equal(t, 1, probeSecretBackend(&buf, fakeLogger{}, backend, "probe-value"),
		"a wallet that loses what it was given has failed")
	assert.Contains(t, buf.String(), "not found after storing it",
		"and it must say the entry was gone, not that it came back wrong")
}

func TestProbeSecretBackendDeleteFails(t *testing.T) {
	backend := &fakeProbeBackend{lookupVal: "probe-value", lookupOK: true, deleteErr: errors.New("boom")}
	var buf bytes.Buffer

	assert.Equal(t, 1, probeSecretBackend(&buf, fakeLogger{}, backend, "probe-value"),
		"a probe entry left behind in the wallet is not a clean run")
	assert.Contains(t, buf.String(), "delete: FAILED", "and the step that failed must be named")
}

func TestProbeSecretBackendUnlockFails(t *testing.T) {
	backend := &fakeProbeBackend{unlockErr: errors.New("boom")}
	session := fakeProbeSession{backend}
	var buf bytes.Buffer

	assert.Equal(t, 1, probeSecretBackend(&buf, fakeLogger{}, session, "probe-value"),
		"a wallet that would not unlock has failed")
	out := buf.String()
	assert.Contains(t, out, "unlock: FAILED", "the step that failed must be named")
	assert.NotContains(t, out, "store:",
		"storing into a wallet that never opened would report a second failure about the first")
}

func TestProbeSecretBackendUnlocksAndLocks(t *testing.T) {
	backend := &fakeProbeBackend{lookupVal: "probe-value", lookupOK: true}
	session := fakeProbeSession{backend}
	var buf bytes.Buffer

	require.Zero(t, probeSecretBackend(&buf, fakeLogger{}, session, "probe-value"),
		"a wallet that opens and does its work has passed")
	assert.Contains(t, buf.String(), "unlock: ok", "the report must say the wallet was opened")
	assert.Equal(t, 1, backend.lockCalls,
		"a wallet this probe opened must be left locked again, exactly once")
}

// keysSecretBackendAssertion pins probeSecretBackend's parameter type against
// keys.SecretBackend so a future interface change here is caught at compile
// time, not by a silent test skip.
var _ keys.SecretBackend = (*fakeProbeBackend)(nil)
