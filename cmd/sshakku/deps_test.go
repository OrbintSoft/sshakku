package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys"
	"github.com/OrbintSoft/sshakku/internal/paths"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// depsReturning builds a deps whose newSecret always yields backend, so a
// command body can be exercised against a fake secret store without opening
// the real D-Bus/CLI-backed one. The other seams keep their production wiring
// (harmless for the secret-store paths this helper serves).
func depsReturning(backend keys.SecretBackend) deps {
	d := realDeps()
	d.newSecret = func(string, keys.Logger, config.Settings) (keys.SecretBackend, func()) {
		return backend, func() {}
	}
	return d
}

// memoryBackend is an in-process keys.SecretBackend that actually records what
// is stored, so a store-then-lookup round-trip (testSecretBackend's probe) and
// a list-then-delete sweep (forget --all) behave like a real wallet without one.
type memoryBackend struct{ stored map[string]string }

func newMemoryBackend() *memoryBackend { return &memoryBackend{stored: map[string]string{}} }

func (m *memoryBackend) Lookup(service string) (string, bool, error) {
	v, ok := m.stored[service]
	return v, ok, nil
}

func (m *memoryBackend) Store(service, _, passphrase string) error {
	m.stored[service] = passphrase
	return nil
}
func (m *memoryBackend) Delete(service string) error { delete(m.stored, service); return nil }
func (m *memoryBackend) List() ([]string, error) {
	names := make([]string, 0, len(m.stored))
	for s := range m.stored {
		names = append(names, s)
	}
	sort.Strings(names)
	return names, nil
}

var _ keys.SecretBackend = (*memoryBackend)(nil)

// TestTestSecretBackend covers testSecretBackend end to end against a fake
// backend: the store/lookup/delete round-trip passes with a recording backend
// (and an empty name defaults to the configured one), while a backend whose
// store fails reports an overall failure.
func TestTestSecretBackend(t *testing.T) {
	t.Run("pass, name defaults to configured backend", func(t *testing.T) {
		d := depsReturning(newMemoryBackend())
		var out, errOut bytes.Buffer
		// ConfigDir has no config.toml, so the backend name resolves to the
		// built-in default; an empty name argument must fall back to it.
		require.Zerof(t, d.testSecretBackend(t.Context(), &out, &errOut, paths.Layout{ConfigDir: t.TempDir()}, fakeLogger{}, ""),
			"a wallet that stores and reads back must pass; stderr=%q", errOut.String())
		assert.Contains(t, out.String(), "backend: "+config.DefaultSecretBackend(),
			"with no name given, the wallet tested is the configured one, and the report says which")
		assert.Contains(t, out.String(), "backend test: PASS", "and says whether it worked")
	})

	t.Run("fail when the backend's store fails", func(t *testing.T) {
		d := depsReturning(&fakeProbeBackend{storeErr: errors.New("boom")})
		var out, errOut bytes.Buffer
		assert.Equal(t, 1, d.testSecretBackend(t.Context(), &out, &errOut, paths.Layout{ConfigDir: t.TempDir()}, fakeLogger{}, "keychain"),
			"a wallet that cannot store must not be reported as working")
		assert.Contains(t, out.String(), "backend: keychain", "the wallet named is the one tested")
		assert.Contains(t, out.String(), "backend test: FAIL", "and the verdict must be plain")
	})
}

// TestAskpassBroker covers the wallet-hit path: a key-passphrase prompt whose
// key the fake wallet knows is answered from the wallet, silently, without ever
// touching the terminal. HOME/XDG_STATE_HOME point at a temp dir so the resolved
// layout and its session log stay off the real state dir. The wallet-miss and
// write-error branches fall through to the real /dev/tty, whose availability is
// environment-dependent, so they are left to integration coverage.
func TestAskpassBroker(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_STATE_HOME", tmp)

	d := depsReturning(&fakeProbeBackend{lookupVal: "wallet-pass", lookupOK: true})
	var out bytes.Buffer
	prompt := "Enter passphrase for key '/home/u/.ssh/id_ed25519': "
	require.Zero(t, d.askpassBroker(t.Context(), &out, []string{prompt}), "a passphrase the wallet holds must be answered from it")
	assert.Equal(t, "wallet-pass\n", out.String(),
		"the stored passphrase, with the newline ssh expects and nothing else")
}

// TestLoadKeys covers loadKeys' wiring against a fake backend, in a headless
// session (no display) with a temp HOME so it never execs a prompter or ssh-add
// and never touches the real ~/.ssh. With no ~/.ssh at all the enumerator finds
// no keys and the load is a silent success; a ~/.ssh that is a plain file makes
// enumeration fail, which surfaces as a non-zero exit.
func TestLoadKeys(t *testing.T) {
	t.Run("no keys is a silent success", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("HOME", tmp)
		t.Setenv("XDG_STATE_HOME", tmp)
		t.Setenv("WAYLAND_DISPLAY", "")
		t.Setenv("DISPLAY", "")

		d := depsReturning(newMemoryBackend())
		var errOut bytes.Buffer
		assert.Zerof(t, d.loadKeys(t.Context(), &errOut), "a directory with no key in it is nothing to complain about; stderr=%q", errOut.String())
	})

	t.Run("enumeration failure returns non-zero", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("HOME", tmp)
		t.Setenv("XDG_STATE_HOME", tmp)
		t.Setenv("WAYLAND_DISPLAY", "")
		t.Setenv("DISPLAY", "")
		// A plain file where ~/.ssh should be a directory makes the key
		// enumeration fail with a non-"not exist" error.
		require.NoError(t, os.WriteFile(filepath.Join(tmp, ".ssh"), []byte("not a dir"), 0o600),
			"seed a file where ~/.ssh should be a directory")

		d := depsReturning(newMemoryBackend())
		var errOut bytes.Buffer
		assert.Equal(t, 1, d.loadKeys(t.Context(), &errOut),
			"a key directory that could not be read is not the same as one holding no keys")
	})
}

// TestRunLoadKeys covers run's load-keys case: it dispatches to d.loadKeys with
// the injected backend. Headless with a temp HOME and no ~/.ssh, the load is a
// silent no-op, so this exercises the dispatch wiring without a subprocess.
func TestRunLoadKeys(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_STATE_HOME", tmp)
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", "")

	d := depsReturning(newMemoryBackend())
	assert.Zero(t, d.run(t.Context(), io.Discard, io.Discard, []string{"load-keys"}),
		"load-keys with nothing to load is a silent success")
}

// TestForget covers forget against a fake backend: named keys delete the
// prefixed service and report it, --all lists then deletes every managed entry,
// and a delete failure returns a non-zero code. HOME/XDG_STATE_HOME point at a
// temp dir so the resolved layout stays off the real state dir.
func TestForget(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_STATE_HOME", tmp)

	t.Run("named key", func(t *testing.T) {
		d := depsReturning(newMemoryBackend())
		var out, errOut bytes.Buffer
		require.Zerof(t, d.forget(t.Context(), &out, &errOut, []string{"id_rsa"}), "forget id_rsa; stderr=%q", errOut.String())
		assert.Contains(t, out.String(), "forgot "+keys.DefaultServicePrefix+"-id_rsa",
			"the entry that was removed must be named, under the prefix it was stored with")
	})

	t.Run("--all lists then deletes every managed entry", func(t *testing.T) {
		backend := newMemoryBackend()
		backend.stored[keys.DefaultServicePrefix+"-a"] = "x"
		backend.stored[keys.DefaultServicePrefix+"-b"] = "y"
		d := depsReturning(backend)
		var out, errOut bytes.Buffer
		require.Zerof(t, d.forget(t.Context(), &out, &errOut, []string{"--all"}), "forget --all; stderr=%q", errOut.String())
		assert.Empty(t, backend.stored, "--all must leave nothing of ours behind in the wallet")
	})

	t.Run("delete failure returns non-zero", func(t *testing.T) {
		d := depsReturning(&fakeProbeBackend{deleteErr: errors.New("boom")})
		var out, errOut bytes.Buffer
		assert.Equal(t, 1, d.forget(t.Context(), &out, &errOut, []string{"id_rsa"}),
			"a passphrase still in the wallet must not be reported as forgotten")
	})
}
