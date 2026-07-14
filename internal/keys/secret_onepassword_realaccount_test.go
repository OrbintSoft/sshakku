//go:build unix

package keys

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
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
// see .github/workflows/tier2-onepassword.yml and PLAN.md 4.2). Unlike
// ksecretd / gnome-keyring-daemon / KeePassXC, a 1Password account is a
// cloud account, not a disposable local daemon a container can stand up
// from nothing — the CI job authenticates as a dedicated service account
// instead of a tier-2-style container.
const allowRealOnePasswordEnv = "SSHAKKU_TEST_ALLOW_REAL_ONEPASSWORD"

// opSetupTimeout bounds each vault create/delete call made directly by the
// test (outside OnePasswordBackend) in case op ever prompts interactively
// for confirmation: with no terminal attached, a read on stdin returns EOF
// immediately rather than hanging, but the bound is kept as a safety net
// against an unexpected different failure mode.
const opSetupTimeout = 30 * time.Second

// TestOnePasswordBackendRealAccount exercises OnePasswordBackend end to end
// against a real 1Password account via the op CLI — unlike
// secret_onepassword_test.go, which only ever talks to a fake Runner. It
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
		t.Skipf("skipping: set %s=1 to run against a real, already-authenticated 1Password account (local only — see PLAN.md 4.2)", allowRealOnePasswordEnv)
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
	if err != nil {
		t.Fatalf("op vault create: %v: %s", err, createOut)
	}
	var vault struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(createOut), &vault); err != nil || vault.ID == "" {
		t.Fatalf("parsing op vault create output: %v: %s", err, createOut)
	}
	t.Cleanup(func() {
		if out, err := opRun(t, "vault", "delete", vault.ID); err != nil {
			t.Logf("cleanup: op vault delete %s failed, remove it by hand: %v: %s", vault.ID, err, out)
		}
	})

	backend := &OnePasswordBackend{Runner: ExecRunner{}, Vault: vault.ID}

	const (
		testService = "sshakku-integration-test-probe"
		testLabel   = "sshakku integration test probe"
		testPass    = "probe-passphrase-not-a-real-secret"
	)

	if err := backend.Store(testService, testLabel, testPass); err != nil {
		t.Fatalf("Store: %v", err)
	}

	got, found, err := backend.Lookup(testService)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if !found {
		t.Fatal("Lookup: not found immediately after Store")
	}
	if got != testPass {
		t.Fatalf("Lookup passphrase = %q, want %q", got, testPass)
	}

	services, err := backend.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !containsString(services, testService) {
		t.Fatalf("List = %v, want it to contain %q", services, testService)
	}

	if err := backend.Delete(testService); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, found, err := backend.Lookup(testService); err != nil {
		t.Fatalf("Lookup after Delete: %v", err)
	} else if found {
		t.Fatal("Lookup after Delete: still found")
	}
}

// opRun runs the op CLI directly (not through a Runner) for test setup and
// teardown that is outside what OnePasswordBackend itself does — creating
// and deleting the throwaway vault.
func opRun(t *testing.T, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), opSetupTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, onePasswordBin, args...).CombinedOutput()
	return string(out), err
}

// containsString reports whether s is present in list.
func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
