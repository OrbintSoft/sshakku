//go:build unix

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These name a wallet — KeePassXC — rather than only asking which one would be
// used, so they need this system to have one that can be named at all. Every
// unix does; Windows does not yet, and naming one there is refused by F26
// rather than accepted and found wanting. See
// internal/config/wallet_unix_test.go for what is missing and why these are
// held to this family rather than rewritten around it.
//
// They verify F25 alongside the rest of walletcheck_test.go, and share its
// configuredWallet and onlyOnPath.

func TestDoctorNamesTheWalletAndWhatIsMissing(t *testing.T) {
	d := configuredWallet(t, "secret_backend = \"keepassxc\"\nkeepassxc_route = \"cli\"\nkeepassxc_database = \"/nowhere/vault.kdbx\"\n")
	onlyOnPath(t)

	var out, errOut bytes.Buffer
	require.Zerof(t, d.doctor(t.Context(), &out, &errOut, nil), "a report changes nothing and cannot fail; stderr=%q", errOut.String())
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
	require.Zerof(t, d.doctor(t.Context(), &out, &errOut, nil), "a report changes nothing and cannot fail; stderr=%q", errOut.String())
	report := out.String()

	assert.Contains(t, report, "keepassxc-cli", "the report must name the tool it found")
	assert.NotContains(t, report, "missing", "nothing may be called missing when everything is there")
}

// TestEveryChoosableWalletCanBeDiagnosed holds every name a user may put in
// secret_backend to being a name the diagnostics accept too: a wallet you can
// choose but cannot ask about is one you cannot diagnose when it stops
// working. The names come from the one list rather than being written out
// again here, which is what stopped them agreeing before.
func TestEveryChoosableWalletCanBeDiagnosed(t *testing.T) {
	names := config.SecretBackends()
	require.NotEmpty(t, names, "this system offers no wallet at all")
	for _, name := range names {
		assert.Truef(t, config.SecretBackendAvailable(name),
			"%q can be chosen, so it must be one the diagnostics accept", name)
	}
	assert.False(t, config.SecretBackendAvailable("bogus"), "a name nobody offers must not be accepted")
	assert.Truef(t, config.SecretBackendAvailable(config.DefaultSecretBackend()),
		"the default wallet %q must be one this system offers", config.DefaultSecretBackend())
}
