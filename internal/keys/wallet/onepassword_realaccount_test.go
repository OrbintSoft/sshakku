package wallet

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/run"
)

// allowRealOnePasswordEnv opts this test into creating and deleting a real
// vault against whatever 1Password account the op CLI is currently
// authenticated as. There is no way to tell a disposable account from a
// real one from inside the test, so this must default to skipped, and op
// must already be authenticated before setting it — this test never runs
// `op signin`, never accepts a credential as input, and never logs or
// prints one. Two ways to satisfy that: a developer's own already-signed-in
// session (app integration or `op signin`, local only), or
// OP_SERVICE_ACCOUNT_TOKEN set to a dedicated service account's token (CI —
// see .github/workflows/onepassword-real-account.yml). Unlike
// ksecretd / gnome-keyring-daemon / KeePassXC, a 1Password account is a
// cloud account, not a disposable local daemon a container can stand up
// from nothing — the CI job authenticates as a dedicated service account
// instead of standing up a container.
const allowRealOnePasswordEnv = "SSHAKKU_TEST_ALLOW_REAL_ONEPASSWORD"

// opSetupTimeout bounds each vault create/delete call made directly by the
// test (outside OnePassword) in case op ever prompts interactively
// for confirmation: with no terminal attached, a read on stdin returns EOF
// immediately rather than hanging, but the bound is kept as a safety net
// against an unexpected different failure mode.
const opSetupTimeout = 30 * time.Second

// TestOnePasswordBackendRealAccount exercises OnePassword end to end
// against a real 1Password account via the op CLI — unlike
// secret_onepassword_test.go, which only ever talks to a fake run.Runner. It
// creates its own throwaway vault, named with a timestamp so it can never
// collide with or touch an existing one, runs the backend's Store / Lookup /
// Delete / List against only that vault, and deletes the vault when the test
// ends regardless of outcome — leaving no trace in the account.
//
// op's authentication is live external state that go test's cache has no way
// to see (it isn't a file or an env var), so a second run with the same
// allowRealOnePasswordEnv value can replay a stale cached skip/pass from
// before you signed in. Pass -count=1 to force a real run.

func TestOnePasswordBackendRealAccount(t *testing.T) {
	if os.Getenv(allowRealOnePasswordEnv) == "" {
		t.Skipf("skipping: set %s=1 to run against a real, already-authenticated 1Password account — this creates and deletes a vault in whichever account op is signed in to", allowRealOnePasswordEnv)
	}
	if _, err := exec.LookPath(onePasswordBin); err != nil {
		t.Skipf("op CLI not found: %v", err)
	}
	// op whoami and op signin are both unsupported for service accounts, so
	// `op user get --me` is the one authentication check that works the same
	// way for a developer's session and for OP_SERVICE_ACCOUNT_TOKEN in CI.
	if out, err := opRun(t, "user", "get", "--me"); err != nil {
		t.Skipf("op is not authenticated — sign in yourself first (op signin, or the desktop app integration), or set OP_SERVICE_ACCOUNT_TOKEN: %s", strings.TrimSpace(out))
	}

	vaultName := "sshakku-integration-test-" + time.Now().UTC().Format("20060102T150405.000000000")
	createOut, err := opRun(t, "vault", "create", vaultName, "--format", "json")
	require.NoErrorf(t, err, "the throwaway vault this test works in could not be created: %s", createOut)
	var vault struct {
		ID string `json:"id"`
	}
	require.NoErrorf(t, json.Unmarshal([]byte(createOut), &vault), "op vault create answered with: %s", createOut)
	require.NotEmptyf(t, vault.ID, "op vault create named no vault: %s", createOut)
	t.Cleanup(func() {
		if out, err := opRun(t, "vault", "delete", vault.ID); err != nil {
			t.Logf("cleanup: op vault delete %s failed, remove it by hand: %v: %s", vault.ID, err, out)
		}
	})

	backend := &OnePassword{Runner: run.ExecRunner{}, Vault: vault.ID}

	const (
		testService = "sshakku-integration-test-probe"
		testLabel   = "sshakku integration test probe"
		testPass    = "probe-passphrase-not-a-real-secret"
	)

	require.NoError(t, backend.Store(t.Context(), testService, testLabel, testPass), "saving a passphrase must succeed")

	got, found, err := backend.Lookup(t.Context(), testService)
	require.NoError(t, err, "reading it straight back must succeed")
	require.True(t, found, "a passphrase just saved must be there")
	assert.Equal(t, testPass, got, "and be the one that was saved")

	services, err := backend.List(t.Context())
	require.NoError(t, err, "listing the vault must succeed")
	assert.Contains(t, services, testService, "what SSHakku stored must be reported")

	require.NoError(t, backend.Delete(t.Context(), testService), "forgetting a passphrase must succeed")
	_, found, err = backend.Lookup(t.Context(), testService)
	require.NoError(t, err, "looking for a forgotten passphrase must not be an error")
	assert.False(t, found, "and it must be gone from the vault")
}

// opRun runs the op CLI directly (not through a run.Runner) for test setup and
// teardown that is outside what OnePassword itself does — creating
// and deleting the throwaway vault.
func opRun(t *testing.T, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), opSetupTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, onePasswordBin, args...).CombinedOutput()
	return string(out), err
}
