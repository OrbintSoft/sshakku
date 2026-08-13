package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/keys"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, os.MkdirAll(configDir, 0o700), "make the config dir")
	body := "service_prefix = " + strconv.Quote(prefix) + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(body), 0o600), "write config.toml")
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
	require.Zerof(t, d.forget(t.Context(), &out, &errOut, []string{"id_rsa"}), "forget id_rsa; stderr=%q", errOut.String())
	assert.NotContains(t, backend.stored, prefix+"-id_rsa",
		"the entry the configured prefix names is the one that must be gone from the wallet")
	assert.Contains(t, out.String(), "forgot "+prefix+"-id_rsa",
		"and the report must name it — a report naming an entry it never deleted is the failure this guards against")
}
