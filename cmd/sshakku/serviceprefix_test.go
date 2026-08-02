package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/keys"
)

// configuredPrefix writes a config.toml choosing prefix as the name SSHakku's
// wallet entries carry, points the environment at it, and returns a deps whose
// secret backend is the one given. Only the wallet and the configuration's
// location are stood in for; which entry name a command addresses is what these
// tests judge, and nothing here supplies it.
func configuredPrefix(t *testing.T, prefix string, backend keys.SecretBackend) deps {
	t.Helper()

	root := t.TempDir()
	configDir := filepath.Join(root, "config", "sshakku")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("make the config dir: %v", err)
	}
	body := "service_prefix = " + strconv.Quote(prefix) + "\n"
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(body), 0o600); err != nil {
		t.Fatalf("write config.toml: %v", err)
	}
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))

	return depsReturning(backend)
}

// TestForgetRemovesTheEntryTheConfiguredPrefixNames verifies F32: the entry
// name a wallet carries is the configuration's to choose, and forgetting a key
// addresses that name. The entry is checked in the store itself rather than the
// command's report being believed — a report naming an entry it never deleted
// is exactly the failure this guards against, and the two are asserted
// separately so a run says which of them went wrong.
func TestForgetRemovesTheEntryTheConfiguredPrefixNames(t *testing.T) {
	const prefix = "wallet-of-mine"

	backend := newMemoryBackend()
	backend.stored[prefix+"-id_rsa"] = "the passphrase"
	d := configuredPrefix(t, prefix, backend)

	var out, errOut bytes.Buffer
	if got := d.forget(&out, &errOut, []string{"id_rsa"}); got != 0 {
		t.Fatalf("forget id_rsa = %d, want 0; stderr=%q", got, errOut.String())
	}
	if _, still := backend.stored[prefix+"-id_rsa"]; still {
		t.Errorf("stored = %v, want %q gone", backend.stored, prefix+"-id_rsa")
	}
	if want := "forgot " + prefix + "-id_rsa"; !strings.Contains(out.String(), want) {
		t.Errorf("output = %q, want it to name %q", out.String(), want)
	}
}
