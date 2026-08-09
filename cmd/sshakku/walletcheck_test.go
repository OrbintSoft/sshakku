package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/diagnose"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These verify F25 (docs/FEATURES.md): the doctor names the wallet it would
// use, and how it would reach it, and says when something that wallet needs is
// not there — without touching a stored passphrase to find out.
//
// They drive the real doctor over a real config file, with a real PATH that
// does or does not carry the tool. The agent side of the report is injected,
// because that is a different question entirely and upstream of this one; the
// wallet answer itself is never supplied to the code that is supposed to
// produce it.

// configuredWallet writes a config.toml naming the backend and route, points
// the environment at it, and returns a doctor whose agent report is empty.
func configuredWallet(t *testing.T, config string) deps {
	t.Helper()

	root := t.TempDir()
	configDir := filepath.Join(root, "config", "sshakku")
	require.NoError(t, os.MkdirAll(configDir, 0o700), "make the config dir")
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(config), 0o600), "write config.toml")
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(root, "run"))

	return doctorDeps(diagnose.Report{}, fakeTokenSource{}, 1000)
}

// onlyOnPath replaces PATH with a directory holding exactly the named
// executables, so what the doctor can find is decided here and not by whatever
// the machine running the test happens to have installed.
func onlyOnPath(t *testing.T, names ...string) {
	t.Helper()

	dir := t.TempDir()
	for _, name := range names {
		script := filepath.Join(dir, name)
		require.NoErrorf(t, os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o755), "write %s", script)
	}
	t.Setenv("PATH", dir)
}

func TestDoctorNamesTheWalletAndWhatIsMissing(t *testing.T) {
	d := configuredWallet(t, "secret_backend = \"keepassxc\"\nkeepassxc_route = \"cli\"\nkeepassxc_database = \"/nowhere/vault.kdbx\"\n")
	onlyOnPath(t)

	var out, errOut bytes.Buffer
	require.Zerof(t, d.doctor(&out, &errOut, nil), "a report changes nothing and cannot fail; stderr=%q", errOut.String())
	report := out.String()

	for _, want := range []string{"keepassxc", "cli", "keepassxc-cli", "missing"} {
		assert.Containsf(t, report, want, "nothing tells the user what is wrong without %q", want)
	}
	assert.Contains(t, report, "/nowhere/vault.kdbx", "the report must name the database it could not find")
}

func TestDoctorSaysNothingIsMissingWhenNothingIs(t *testing.T) {
	database := filepath.Join(t.TempDir(), "vault.kdbx")
	require.NoError(t, os.WriteFile(database, []byte("not really a database"), 0o600), "write the database file")
	d := configuredWallet(t, "secret_backend = \"keepassxc\"\nkeepassxc_route = \"cli\"\nkeepassxc_database = \""+database+"\"\n")
	onlyOnPath(t, "keepassxc-cli")

	var out, errOut bytes.Buffer
	require.Zerof(t, d.doctor(&out, &errOut, nil), "a report changes nothing and cannot fail; stderr=%q", errOut.String())
	report := out.String()

	assert.Contains(t, report, "keepassxc-cli", "the report must name the tool it found")
	assert.NotContains(t, report, "missing", "nothing may be called missing when everything is there")
}

// TestDoctorNamesTheWalletThatWouldBeUsed verifies F25 through the whole
// command, for the machine most users have: one nobody has configured. Knowing
// which wallet was meant is half the answer to "why was nothing saved", and the
// report is where a user looks for it.
//
// The expected name comes from the configuration layer rather than being
// written out, because which wallet an unconfigured machine uses is that
// layer's answer to give, and it differs per operating system.
func TestDoctorNamesTheWalletThatWouldBeUsed(t *testing.T) {
	d := configuredWallet(t, "")
	onlyOnPath(t)

	var out, errOut bytes.Buffer
	require.Zerof(t, d.doctor(&out, &errOut, nil), "a report changes nothing and cannot fail; stderr=%q", errOut.String())
	assert.Containsf(t, out.String(), config.DefaultSecretBackend(),
		"the report must name %q, the wallet an unconfigured machine would open", config.DefaultSecretBackend())
}

// TestDoctorRefusesAWalletThisSystemHasNot verifies F26 through the command: a
// wallet named in the configuration that this operating system does not have is
// a mistake in the configuration, so the report names the wallet actually in
// use rather than the one that was asked for.
func TestDoctorRefusesAWalletThisSystemHasNot(t *testing.T) {
	absent := "keychain"
	if config.SecretBackendAvailable(absent) {
		absent = "secret-service"
	}
	d := configuredWallet(t, "secret_backend = \""+absent+"\"\n")
	onlyOnPath(t)

	var out, errOut bytes.Buffer
	require.Zerof(t, d.doctor(&out, &errOut, nil), "a report changes nothing and cannot fail; stderr=%q", errOut.String())
	report := out.String()
	assert.NotContainsf(t, report, "backend:               "+absent,
		"the report must not name %q, a wallet this system has not got", absent)
	assert.Containsf(t, report, config.DefaultSecretBackend(),
		"it must name %q, the wallet actually in use", config.DefaultSecretBackend())
}
