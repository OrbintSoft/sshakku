package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/keys"
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
		if got := d.askpassEnv(&out, &errOut); got != 0 {
			t.Fatalf("askpassEnv = %d, want 0; stderr=%q", got, errOut.String())
		}
		s := out.String()
		for _, want := range []string{
			"export SSH_ASKPASS='/opt/sshakku/bin/sshakku-askpass'",
			"export SSH_ASKPASS_REQUIRE=force",
		} {
			if !strings.Contains(s, want) {
				t.Errorf("output %q missing %q", s, want)
			}
		}
	})

	t.Run("executable lookup failure returns 1", func(t *testing.T) {
		d := realDeps()
		d.self = func() (string, error) { return "", errors.New("no exe") }
		var out, errOut bytes.Buffer
		if got := d.askpassEnv(&out, &errOut); got != 1 {
			t.Fatalf("askpassEnv (exe lookup fails) = %d, want 1", got)
		}
		if out.Len() != 0 {
			t.Errorf("stdout = %q, want no exports when the lookup fails", out.String())
		}
		if !strings.Contains(errOut.String(), "no exe") {
			t.Errorf("stderr = %q, want the lookup error", errOut.String())
		}
	})

	t.Run("stdout write error returns 1", func(t *testing.T) {
		d := realDeps()
		d.self = func() (string, error) { return "/opt/sshakku/bin/sshakku", nil }
		var errOut bytes.Buffer
		if got := d.askpassEnv(errWriter{}, &errOut); got != 1 {
			t.Errorf("askpassEnv (failing stdout) = %d, want 1", got)
		}
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
		if got := d.askpassFromHandoff(&out); got != 0 {
			t.Fatalf("askpassFromHandoff = %d, want 0", got)
		}
		if out.String() != "s3cret\n" {
			t.Errorf("output = %q, want the passphrase with a trailing newline", out.String())
		}
	})

	t.Run("fetch error returns 1", func(t *testing.T) {
		d := realDeps()
		d.fetchHandoff = func(string) (string, error) { return "", errors.New("boom") }
		if got := d.askpassFromHandoff(&bytes.Buffer{}); got != 1 {
			t.Errorf("askpassFromHandoff (fetch fails) = %d, want 1", got)
		}
	})

	t.Run("empty passphrase returns 1", func(t *testing.T) {
		d := realDeps()
		d.fetchHandoff = func(string) (string, error) { return "", nil }
		if got := d.askpassFromHandoff(&bytes.Buffer{}); got != 1 {
			t.Errorf("askpassFromHandoff (empty passphrase) = %d, want 1", got)
		}
	})

	t.Run("stdout write error returns 1", func(t *testing.T) {
		d := realDeps()
		d.fetchHandoff = func(string) (string, error) { return "s3cret", nil }
		if got := d.askpassFromHandoff(errWriter{}); got != 1 {
			t.Errorf("askpassFromHandoff (failing stdout) = %d, want 1", got)
		}
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
		if got := d.askpass(&out, nil); got != 0 {
			t.Fatalf("askpass (handoff token set) = %d, want 0", got)
		}
		if out.String() != "from-handoff\n" {
			t.Errorf("output = %q, want the handoff passphrase", out.String())
		}
	})

	t.Run("no token falls through to the wallet broker", func(t *testing.T) {
		t.Setenv(keys.EnvPassHandoffToken, "")
		d := depsReturning(&fakeProbeBackend{lookupVal: "wallet-pass", lookupOK: true})
		var out bytes.Buffer
		prompt := "Enter passphrase for key '/home/u/.ssh/id_ed25519': "
		if got := d.askpass(&out, []string{prompt}); got != 0 {
			t.Fatalf("askpass (wallet hit) = %d, want 0", got)
		}
		if out.String() != "wallet-pass\n" {
			t.Errorf("output = %q, want the wallet passphrase", out.String())
		}
	})
}
