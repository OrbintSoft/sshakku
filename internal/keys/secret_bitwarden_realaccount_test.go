//go:build unix

package keys

import (
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/run"
)

// allowRealBitwardenEnv opts this test into driving a real bw CLI through
// BitwardenBackend's own Unlock/Lock — including the master-password prompt
// — against a real Bitwarden (or self-hosted Vaultwarden) account. Unlike
// the 1Password real-account test, there is no OS-integrated unlock to rely
// on (see the design note on BitwardenBackend), so this test needs the
// account's identity up front: SSHAKKU_TEST_BW_EMAIL and
// SSHAKKU_TEST_BW_PASSWORD (a fixed, disposable-fixture password — never a
// real secret; see vaultwarden-session.sh) and, for a self-hosted
// server, SSHAKKU_TEST_BW_SERVER. It never runs `op signin`-equivalent setup
// itself beyond that: Unlock is the exact production code path, exercised
// here for real rather than assumed correct because it round-trips through
// a fake run.Runner in secret_bitwarden_test.go. A Bitwarden/Vaultwarden account
// needs a server to talk to, unlike the local `op` app-integration case, so
// something has to stand one up: the container's own session script on Linux,
// or test/vaultwarden-server.sh, which puts the server in a container and
// leaves this test and the bw CLI outside it.
const allowRealBitwardenEnv = "SSHAKKU_TEST_ALLOW_REAL_BITWARDEN"

// TestBitwardenBackendRealAccount exercises BitwardenBackend end to end
// against a real bw CLI, driving Unlock/Lock itself (via a fixed-answer
// prompt.Prompter, never a real interactive one) rather than receiving an
// already-unlocked session — unlike secret_bitwarden_test.go, which only
// ever talks to a fake run.Runner. It creates its own throwaway item, named
// with a timestamp so it can never collide with or touch an existing one,
// and deletes it in t.Cleanup regardless of outcome.
func TestBitwardenBackendRealAccount(t *testing.T) {
	if os.Getenv(allowRealBitwardenEnv) == "" {
		t.Skipf("skipping: set %s=1 plus SSHAKKU_TEST_BW_EMAIL/SSHAKKU_TEST_BW_PASSWORD (and optionally SSHAKKU_TEST_BW_SERVER) to run against a real bw account; test/vaultwarden-server.sh sets all of them up around a disposable server", allowRealBitwardenEnv)
	}
	if _, err := exec.LookPath(bitwardenBin); err != nil {
		t.Skipf("bw CLI not found: %v", err)
	}
	email := os.Getenv("SSHAKKU_TEST_BW_EMAIL")
	password := os.Getenv("SSHAKKU_TEST_BW_PASSWORD")
	if email == "" || password == "" {
		t.Skip("SSHAKKU_TEST_BW_EMAIL and SSHAKKU_TEST_BW_PASSWORD must both be set")
	}

	backend := &BitwardenBackend{
		Runner:   run.ExecRunner{},
		Prompter: &fakePrompter{pass: password},
		Email:    email,
		Server:   os.Getenv("SSHAKKU_TEST_BW_SERVER"),
	}

	// The probe is named the way sshakku names what it stores, since that is
	// what the backend is being asked to handle: a name of some other shape
	// would be a different item than any real one.
	stamp := time.Now().UTC().Format("20060102T150405.000000000")
	testService := DefaultServicePrefix + "-integration-test-probe-" + stamp
	// foreignService imitates what else lives in a vault someone actually
	// uses — a saved password that has nothing to do with sshakku.
	foreignService := "not-sshakku-integration-test-probe-" + stamp
	const (
		testLabel = "sshakku integration test probe"
		testPass  = "probe-passphrase-not-a-real-secret"
	)
	t.Cleanup(func() { _ = backend.Delete(t.Context(), testService) })
	t.Cleanup(func() { _ = backend.Delete(t.Context(), foreignService) })

	// Each call below unlocks and locks for itself (held stays false), the
	// same standalone bracket the reactive askpass-broker path uses — so
	// this also proves a *repeated* fresh master-password prompt/unlock
	// works against a real daemon, not just once.
	require.NoError(t, backend.Store(t.Context(), testService, testLabel, testPass), "saving a passphrase must succeed")

	got, found, err := backend.Lookup(t.Context(), testService)
	require.NoError(t, err, "reading it straight back must succeed")
	require.True(t, found, "a passphrase just saved must be there")
	assert.Equal(t, testPass, got, "and be the one that was saved")

	// F27, against the real CLI: `bw list items` answers with the whole vault,
	// so an item sshakku did not store has to be dropped before List returns —
	// whatever List reports is what `forget --all` goes on to delete.
	require.NoError(t, backend.Store(t.Context(), foreignService, "not sshakku's", "someone-elses-password"),
		"the vault must be made to hold something that is not SSHakku's, or there is nothing to leave alone")

	services, err := backend.List(t.Context())
	require.NoError(t, err, "listing the vault must succeed")
	assert.Contains(t, services, testService, "what SSHakku stored must be reported")
	assert.NotContains(t, services, foreignService,
		"and what it did not must not be: whatever is listed here is what forget --all goes on to delete")

	require.NoError(t, backend.Delete(t.Context(), foreignService), "the foreign item must be cleaned up")

	const updatedPass = "probe-passphrase-updated-not-a-real-secret"
	require.NoError(t, backend.Store(t.Context(), testService, testLabel, updatedPass), "replacing a passphrase must succeed")
	got, _, err = backend.Lookup(t.Context(), testService)
	require.NoError(t, err, "reading the replacement back must succeed")
	assert.Equal(t, updatedPass, got, "and it must be the new passphrase, not the one it replaced")

	require.NoError(t, backend.Delete(t.Context(), testService), "forgetting a passphrase must succeed")
	_, found, err = backend.Lookup(t.Context(), testService)
	require.NoError(t, err, "looking for a forgotten passphrase must not be an error")
	assert.False(t, found, "and it must be gone from the vault")
}
