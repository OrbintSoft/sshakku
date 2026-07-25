package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys"
	"github.com/OrbintSoft/sshakku/internal/paths"
)

// depsReturning builds a deps whose newSecret always yields backend, so a
// command body can be exercised against a fake secret store without opening
// the real D-Bus/CLI-backed one.
func depsReturning(backend keys.SecretBackend) deps {
	return deps{
		newSecret: func(string, keys.Logger, config.Settings) (keys.SecretBackend, func()) {
			return backend, func() {}
		},
	}
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
		if got := d.testSecretBackend(&out, &errOut, paths.Layout{ConfigDir: t.TempDir()}, fakeLogger{}, ""); got != 0 {
			t.Fatalf("testSecretBackend = %d, want 0 (pass); stderr=%q", got, errOut.String())
		}
		s := out.String()
		if !strings.Contains(s, "backend: "+config.SecretBackendSecretService) {
			t.Errorf("output = %q, want the default backend name in the header", s)
		}
		if !strings.Contains(s, "backend test: PASS") {
			t.Errorf("output = %q, want an overall PASS", s)
		}
	})

	t.Run("fail when the backend's store fails", func(t *testing.T) {
		d := depsReturning(&fakeProbeBackend{storeErr: errors.New("boom")})
		var out, errOut bytes.Buffer
		if got := d.testSecretBackend(&out, &errOut, paths.Layout{ConfigDir: t.TempDir()}, fakeLogger{}, "keychain"); got != 1 {
			t.Fatalf("testSecretBackend = %d, want 1 (fail)", got)
		}
		s := out.String()
		if !strings.Contains(s, "backend: keychain") || !strings.Contains(s, "backend test: FAIL") {
			t.Errorf("output = %q, want a keychain header and an overall FAIL", s)
		}
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
	if got := d.askpassBroker(&out, []string{prompt}); got != 0 {
		t.Fatalf("askpassBroker (wallet hit) = %d, want 0", got)
	}
	if got := out.String(); got != "wallet-pass\n" {
		t.Errorf("askpassBroker wrote %q, want the wallet passphrase with a trailing newline", got)
	}
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
		if got := d.loadKeys(&errOut); got != 0 {
			t.Fatalf("loadKeys (no keys) = %d, want 0; stderr=%q", got, errOut.String())
		}
	})

	t.Run("enumeration failure returns non-zero", func(t *testing.T) {
		tmp := t.TempDir()
		t.Setenv("HOME", tmp)
		t.Setenv("XDG_STATE_HOME", tmp)
		t.Setenv("WAYLAND_DISPLAY", "")
		t.Setenv("DISPLAY", "")
		// A plain file where ~/.ssh should be a directory makes the key
		// enumeration fail with a non-"not exist" error.
		if err := os.WriteFile(filepath.Join(tmp, ".ssh"), []byte("not a dir"), 0o600); err != nil {
			t.Fatalf("seed ~/.ssh file: %v", err)
		}

		d := depsReturning(newMemoryBackend())
		var errOut bytes.Buffer
		if got := d.loadKeys(&errOut); got != 1 {
			t.Errorf("loadKeys (bad ~/.ssh) = %d, want 1", got)
		}
	})
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
		if got := d.forget(&out, &errOut, []string{"id_rsa"}); got != 0 {
			t.Fatalf("forget id_rsa = %d, want 0; stderr=%q", got, errOut.String())
		}
		if want := keys.DefaultServicePrefix + "-id_rsa"; !strings.Contains(out.String(), "forgot "+want) {
			t.Errorf("output = %q, want a forgot line for %q", out.String(), want)
		}
	})

	t.Run("--all lists then deletes every managed entry", func(t *testing.T) {
		backend := newMemoryBackend()
		backend.stored[keys.DefaultServicePrefix+"-a"] = "x"
		backend.stored[keys.DefaultServicePrefix+"-b"] = "y"
		d := depsReturning(backend)
		var out, errOut bytes.Buffer
		if got := d.forget(&out, &errOut, []string{"--all"}); got != 0 {
			t.Fatalf("forget --all = %d, want 0; stderr=%q", got, errOut.String())
		}
		if len(backend.stored) != 0 {
			t.Errorf("stored = %v, want every entry deleted", backend.stored)
		}
	})

	t.Run("delete failure returns non-zero", func(t *testing.T) {
		d := depsReturning(&fakeProbeBackend{deleteErr: errors.New("boom")})
		var out, errOut bytes.Buffer
		if got := d.forget(&out, &errOut, []string{"id_rsa"}); got != 1 {
			t.Errorf("forget with a failing delete = %d, want 1", got)
		}
	})
}
