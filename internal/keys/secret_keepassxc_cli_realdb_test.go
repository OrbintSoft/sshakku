package keys

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/run"
)

// This exercises the product against a real KeePassXC database, created by the
// real keepassxc-cli, with nothing about the wire format faked.
//
// It verifies features F4, F5/F6 and F9 (docs/FEATURES.md): a passphrase is
// saved on first use, read back afterwards, and removed by forgetting it. The
// scenario is written from those promises — store it, get it back, forget it,
// find it gone — not from what the code does to keep them.
//
// It also answers the one thing that cannot be settled by reading: keepassxc-cli
// has no documented way to take the database password other than by asking, and
// SSHakku feeds it on standard input. If that ever stops working, this is where
// it shows.
//
// Opt-in, never on reachability: an installed keepassxc-cli is not consent to
// create databases on someone's machine.
func TestKeePassXCCLIRealDatabase(t *testing.T) {
	if os.Getenv("SSHAKKU_TEST_ALLOW_REAL_KEEPASSXC") != "1" {
		t.Skip("skipping: set SSHAKKU_TEST_ALLOW_REAL_KEEPASSXC=1 to run against a real keepassxc-cli and a throwaway database")
	}
	_, err := exec.LookPath(keepassxcCLIBin)
	require.NoErrorf(t, err, "%s is what this test drives, and it is not installed", keepassxcCLIBin)

	const dbPassword = "throwaway-database-password"
	const service = defaultServicePrefix + "-id_ed25519"
	const passphrase = "the-key-passphrase"

	db := filepath.Join(t.TempDir(), "throwaway.kdbx")
	createRealDatabase(t, db, dbPassword)

	b := &KeePassXCCLIBackend{
		Runner:   run.ExecRunner{Timeout: 30 * time.Second},
		Prompter: &countingPrompter{password: dbPassword},
		Database: db,
		Timeout:  30 * time.Second,
	}

	// F4: nothing is stored yet, so a lookup misses rather than failing.
	_, found, err := b.Lookup(t.Context(), service)
	require.NoError(t, err, "a database with nothing in it is not an error")
	require.False(t, found, "and a fresh database holds nothing")

	// F4: the passphrase is saved.
	require.NoError(t, b.Store(t.Context(), service, service, passphrase), "saving a passphrase must succeed")

	// F5/F6: it comes back, unchanged.
	got, found, err := b.Lookup(t.Context(), service)
	require.NoError(t, err, "reading it straight back must succeed")
	require.True(t, found, "a passphrase just saved must be there")
	assert.Equal(t, passphrase, got, "and be the one that was saved, byte for byte")

	// Storing again must replace, not accumulate: two entries holding the same
	// secret is one more copy than the user asked for.
	const changed = "a-different-passphrase"
	require.NoError(t, b.Store(t.Context(), service, service, changed), "replacing a passphrase must succeed")
	got, _, err = b.Lookup(t.Context(), service)
	require.NoError(t, err, "reading the replacement back must succeed")
	assert.Equal(t, changed, got, "and it must be the new passphrase, not the one it replaced")
	entries, err := b.List(t.Context())
	require.NoError(t, err, "listing the database must succeed")
	assert.Equal(t, []string{service}, entries,
		"one key is one entry: a second holding the same secret is one more copy than the user asked for")

	// F9: forgetting removes it, and the next use finds nothing.
	require.NoError(t, b.Delete(t.Context(), service), "forgetting a passphrase must succeed")
	_, found, err = b.Lookup(t.Context(), service)
	require.NoError(t, err, "looking for a forgotten passphrase must not be an error")
	assert.False(t, found, "and it must be gone from the database")
}

// createRealDatabase makes a throwaway .kdbx with the real keepassxc-cli.
//
// The password goes on standard input here too — this is the same undocumented
// arrangement the backend relies on, so if it fails, it fails loudly at setup
// rather than looking like a defect in the code under test.
func createRealDatabase(t *testing.T, path, password string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), keepassxcCLIBin, "db-create", "-p", path)
	// db-create asks twice: the password and its confirmation.
	cmd.Stdin = strings.NewReader(password + "\n" + password + "\n")
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "creating a throwaway database failed:\n%s", out)
	_, statErr := os.Stat(path)
	require.NoErrorf(t, statErr, "keepassxc-cli reported success but wrote no database:\n%s", out)
}
