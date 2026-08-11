package main

import (
	"bytes"
	"errors"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/keys"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAskpassEnv covers askpass-env against a fake executable lookup, so it runs
// regardless of where the test binary lives: a resolved path emits the three
// export lines, an os.Executable failure returns 1, and a failing stdout
// surfaces as a write error.
func TestAskpassEnv(t *testing.T) {
	t.Run("resolved path emits the export lines", func(t *testing.T) {
		d := realDeps()
		d.self = func() (string, error) { return "/opt/sshakku/bin/sshakku", nil }
		var out, errOut bytes.Buffer
		require.Zerof(t, d.askpassEnv(&out, &errOut), "askpassEnv; stderr=%q", errOut.String())
		assert.Contains(t, out.String(), "export SSH_ASKPASS='/opt/sshakku/bin/sshakku-askpass'",
			"the helper beside the binary is what ssh must be pointed at, not the binary itself")
		assert.Contains(t, out.String(), "export SSH_ASKPASS_REQUIRE=force",
			"and force is what makes ssh consult it in a session with no display")
	})

	t.Run("executable lookup failure returns 1", func(t *testing.T) {
		d := realDeps()
		d.self = func() (string, error) { return "", errors.New("no exe") }
		var out, errOut bytes.Buffer
		assert.Equal(t, 1, d.askpassEnv(&out, &errOut), "with no path to the binary there is nothing to export")
		assert.Empty(t, out.String(), "and a shell must not be given exports pointing at nothing")
		assert.Contains(t, errOut.String(), "no exe", "the reason must reach the user")
	})

	t.Run("stdout write error returns 1", func(t *testing.T) {
		d := realDeps()
		d.self = func() (string, error) { return "/opt/sshakku/bin/sshakku", nil }
		var errOut bytes.Buffer
		assert.Equal(t, 1, d.askpassEnv(errWriter{}, &errOut),
			"exports the shell never received must not be reported as delivered")
	})
}

// TestAskpassFromHandoff covers the handoff redemption path against a fake
// fetchHandoff, so it runs without a live kernel keyring: a resolved passphrase
// is written to stdout, a fetch error returns 1, an empty passphrase returns 1,
// and a failing stdout surfaces as a write error. HOME and XDG_STATE_HOME point
// at a temp dir so the session log stays off the real state dir.
func TestAskpassFromHandoff(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_STATE_HOME", tmp)
	t.Setenv(keys.EnvPassHandoffToken, "42")

	t.Run("resolved passphrase is written", func(t *testing.T) {
		d := realDeps()
		d.fetchHandoff = func(string) (string, error) { return "s3cret", nil }
		var out bytes.Buffer
		require.Zero(t, d.askpassFromHandoff(&out), "a token that resolves is a prompt that can be answered")
		assert.Equal(t, "s3cret\n", out.String(), "the passphrase, with the newline ssh expects and nothing else")
	})

	t.Run("fetch error returns 1", func(t *testing.T) {
		d := realDeps()
		d.fetchHandoff = func(string) (string, error) { return "", errors.New("boom") }
		assert.Equal(t, 1, d.askpassFromHandoff(&bytes.Buffer{}),
			"a stash that could not be read must be reported, not answered with nothing")
	})

	t.Run("empty passphrase returns 1", func(t *testing.T) {
		d := realDeps()
		d.fetchHandoff = func(string) (string, error) { return "", nil }
		assert.Equal(t, 1, d.askpassFromHandoff(&bytes.Buffer{}),
			"an empty passphrase is not an answer, and handing one to ssh spends a retry")
	})

	t.Run("stdout write error returns 1", func(t *testing.T) {
		d := realDeps()
		d.fetchHandoff = func(string) (string, error) { return "s3cret", nil }
		assert.Equal(t, 1, d.askpassFromHandoff(errWriter{}),
			"a passphrase ssh never received must not be reported as given")
	})
}

// TestAskpassDispatch covers askpass's routing both ways: with the handoff token
// set it redeems the stash, and with no token it falls through to the wallet
// broker (answered here from a fake wallet, never the terminal).
func TestAskpassDispatch(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_STATE_HOME", tmp)

	t.Run("handoff token routes to the stash", func(t *testing.T) {
		t.Setenv(keys.EnvPassHandoffToken, "42")
		d := realDeps()
		d.fetchHandoff = func(string) (string, error) { return "from-handoff", nil }
		var out bytes.Buffer
		require.Zero(t, d.askpass(&out, nil), "a token in the environment is a stash to redeem")
		assert.Equal(t, "from-handoff\n", out.String(), "and the answer comes from it")
	})

	t.Run("no token falls through to the wallet broker", func(t *testing.T) {
		t.Setenv(keys.EnvPassHandoffToken, "")
		d := depsReturning(&fakeProbeBackend{lookupVal: "wallet-pass", lookupOK: true})
		var out bytes.Buffer
		prompt := "Enter passphrase for key '/home/u/.ssh/id_ed25519': "
		require.Zero(t, d.askpass(&out, []string{prompt}), "with no token the wallet is asked")
		assert.Equal(t, "wallet-pass\n", out.String(), "and the answer comes from it, not from the terminal")
	})
}
