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
// The vault is also made to hold items the backend did not create, since a
// vault the user keeps other things in is a vault this backend has to work in
// (F56), and an empty one cannot show whether it leaves anything alone.
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
	if out, err := opRun(t.Context(), t, "user", "get", "--me"); err != nil {
		t.Skipf("op is not authenticated — sign in yourself first (op signin, or the desktop app integration), or set OP_SERVICE_ACCOUNT_TOKEN: %s", strings.TrimSpace(out))
	}

	vaultName := "sshakku-integration-test-" + time.Now().UTC().Format("20060102T150405.000000000")
	createOut, err := opRun(t.Context(), t, "vault", "create", vaultName, "--format", "json")
	require.NoErrorf(t, err, "the throwaway vault this test works in could not be created: %s", createOut)
	var vault struct {
		ID string `json:"id"`
	}
	require.NoErrorf(t, json.Unmarshal([]byte(createOut), &vault), "op vault create answered with: %s", createOut)
	require.NotEmptyf(t, vault.ID, "op vault create named no vault: %s", createOut)
	// The vault outlives the test's own context: Go cancels that one just
	// before the cleanups run, so a delete deriving from it is dead on arrival.
	// WithoutCancel keeps the test's values and drops only the cancellation,
	// and opSetupTimeout still bounds the call.
	//
	// A vault that survives the run is a failure and not a note: it stays in
	// somebody's real account until a person removes it by hand, and there is
	// no later run that will notice.
	t.Cleanup(func() {
		out, err := opRun(context.WithoutCancel(t.Context()), t, "vault", "delete", vault.ID)
		assert.NoErrorf(t, err, "the throwaway vault %s is still in the account and has to be removed by hand: %s", vault.ID, out)
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

	// F56: the vault does not have to be one kept for nothing else — what
	// SSHakku did not put there it neither reads, nor lists, nor removes. Both
	// items below are created with op directly rather than through the backend,
	// which marks everything it creates as its own: made the other way they
	// would carry the mark, and there would be nothing to tell apart.
	const (
		foreignService = "not-sshakku-integration-test-someone-elses"
		foreignPass    = "someone-elses-password-not-a-real-secret"
		// Titled the way SSHakku titles what it stores, which is the case the
		// mark exists for: a shared vault can hold a name that looks like one
		// of ours, and a title is then not enough to tell whose item it is.
		lookalikeService = "sshakku-integration-test-someone-elses"
		lookalikePass    = "someone-elses-password-under-a-familiar-name"
	)
	opCreateItemNotOurs(t.Context(), t, vault.ID, foreignService, foreignPass)
	lookalikeID := opCreateItemNotOurs(t.Context(), t, vault.ID, lookalikeService, lookalikePass)

	services, err = backend.List(t.Context())
	require.NoError(t, err, "listing the vault must succeed")
	assert.NotContains(t, services, foreignService,
		"an item SSHakku did not store must not be listed: what is listed here is what forget --all goes on to delete")
	assert.NotContains(t, services, lookalikeService,
		"and a title that looks like one of SSHakku's must not change that")

	_, found, err = backend.Lookup(t.Context(), lookalikeService)
	require.NoError(t, err, "looking for a passphrase must not be an error")
	assert.False(t, found, "a passphrase SSHakku did not store must not be handed back as one of its own")

	// Saving under a name somebody else's item already carries must not be how
	// that item disappears. What Store answers is not what F56 promises about;
	// the promise is about the item, which has to be where its owner left it
	// whatever SSHakku decided to do with the request.
	storeErr := backend.Store(t.Context(), lookalikeService, testLabel, testPass)
	survived, err := opRun(t.Context(), t, "read", "op://"+vault.ID+"/"+lookalikeID+"/"+onePasswordPasswordField, "--no-newline")
	require.NoErrorf(t, err, "an item SSHakku did not create must still be in the vault, but reading it back failed (Store answered %v): %s", storeErr, survived)
	assert.Equal(t, lookalikePass, strings.TrimSpace(survived), "and it must still hold what its owner put in it")
}

// opCreateItemNotOurs puts an item in vaultID the way its owner would — with op
// itself, carrying none of the marking OnePassword applies to what it creates.
// It answers with the item's ID so that what becomes of it can be checked
// without going through a title something else may by then also be using.
//
// The value travels as an assignment argument, which op warns is visible to
// other processes on the machine. That is the wrong way to handle a real secret
// and is why OnePassword itself does not do it; this one is a literal in this
// file, so there is nothing here to expose.
func opCreateItemNotOurs(ctx context.Context, t *testing.T, vaultID, title, password string) string {
	t.Helper()
	out, err := opRun(ctx, t, onePasswordItemCommand, "create",
		"--category", "password",
		"--title", title,
		onePasswordVaultFlag, vaultID,
		"--format", "json",
		onePasswordPasswordField+"="+password,
	)
	require.NoErrorf(t, err, "the vault must be made to hold something that is not SSHakku's, or there is nothing to leave alone: %s", out)
	var item struct {
		ID string `json:"id"`
	}
	require.NoErrorf(t, json.Unmarshal([]byte(out), &item), "op item create answered with: %s", out)
	require.NotEmptyf(t, item.ID, "op item create named no item: %s", out)
	return item.ID
}

// opRun runs the op CLI directly (not through a run.Runner) for test setup and
// teardown that is outside what OnePassword itself does — creating the
// throwaway vault, putting items in it that the backend did not create, and
// deleting the vault afterwards.
//
// The caller passes the context because the teardown's is not the body's: what
// runs after the test needs one that outlives it.
func opRun(ctx context.Context, t *testing.T, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, opSetupTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, onePasswordBin, args...).CombinedOutput()
	return string(out), err
}
