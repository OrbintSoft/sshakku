package main

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/agent"
	"github.com/OrbintSoft/sshakku/internal/keys"
	"github.com/OrbintSoft/sshakku/internal/paths"
)

// fakeTTY stands in for the /dev/tty prompter so the broker's terminal-fallback
// decisions run without a controlling terminal: it returns a canned reply, or an
// error (e.g. keys.ErrNoTerminal) to drive the decline path.
type fakeTTY struct {
	reply string
	err   error
}

func (f fakeTTY) Prompt(string, bool) (string, error) { return f.reply, f.err }

var _ keys.TTY = fakeTTY{}

// TestForgetSession covers forget's up-front unlock/lock of a session-capable
// backend: a clean session unlocks and re-locks around the sweep, an unlock
// failure is logged but the sweep still proceeds, and a lock failure on the way
// out is logged. HOME/XDG_STATE_HOME point at a temp dir so the session log
// stays off the real state dir.
func TestForgetSession(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_STATE_HOME", tmp)

	t.Run("unlocks and re-locks around the sweep", func(t *testing.T) {
		backend := &fakeProbeBackend{}
		d := depsReturning(fakeProbeSession{backend})
		var out, errOut bytes.Buffer
		if got := d.forget(&out, &errOut, []string{"id_rsa"}); got != 0 {
			t.Fatalf("forget = %d, want 0; stderr=%q", got, errOut.String())
		}
	})

	t.Run("unlock failure still sweeps", func(t *testing.T) {
		backend := &fakeProbeBackend{unlockErr: errors.New("locked")}
		d := depsReturning(fakeProbeSession{backend})
		var out, errOut bytes.Buffer
		if got := d.forget(&out, &errOut, []string{"id_rsa"}); got != 0 {
			t.Fatalf("forget (unlock fails) = %d, want 0; stderr=%q", got, errOut.String())
		}
		if !strings.Contains(out.String(), "forgot ") {
			t.Errorf("output = %q, want the delete to still run after an unlock failure", out.String())
		}
	})

	t.Run("lock failure on exit is tolerated", func(t *testing.T) {
		backend := &fakeProbeBackend{lockErr: errors.New("stuck")}
		d := depsReturning(fakeProbeSession{backend})
		var out, errOut bytes.Buffer
		if got := d.forget(&out, &errOut, []string{"id_rsa"}); got != 0 {
			t.Fatalf("forget (lock fails) = %d, want 0; stderr=%q", got, errOut.String())
		}
	})
}

// TestForgetErrors covers forget's remaining rejections: --all with key names is
// a usage error, an unsupported List (a CLI-only backend) reports the explicit
// hint, and any other List failure reports the raw error. HOME/XDG_STATE_HOME
// point at a temp dir so the session log stays off the real state dir.
func TestForgetErrors(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_STATE_HOME", tmp)

	t.Run("--all with names is a usage error", func(t *testing.T) {
		d := depsReturning(newMemoryBackend())
		var out, errOut bytes.Buffer
		if got := d.forget(&out, &errOut, []string{"--all", "id_rsa"}); got != 2 {
			t.Fatalf("forget --all id_rsa = %d, want 2", got)
		}
		if !strings.Contains(errOut.String(), "cannot be combined") {
			t.Errorf("stderr = %q, want the combine rejection", errOut.String())
		}
	})

	t.Run("--all with an unsupported List reports the hint", func(t *testing.T) {
		d := depsReturning(&fakeProbeBackend{listErr: keys.ErrListUnsupported})
		var out, errOut bytes.Buffer
		if got := d.forget(&out, &errOut, []string{"--all"}); got != 1 {
			t.Fatalf("forget --all (list unsupported) = %d, want 1", got)
		}
		if !strings.Contains(errOut.String(), "native Secret Service backend") {
			t.Errorf("stderr = %q, want the backend hint", errOut.String())
		}
	})

	t.Run("--all with a failing List reports the error", func(t *testing.T) {
		d := depsReturning(&fakeProbeBackend{listErr: errors.New("boom")})
		var out, errOut bytes.Buffer
		if got := d.forget(&out, &errOut, []string{"--all"}); got != 1 {
			t.Fatalf("forget --all (list fails) = %d, want 1", got)
		}
		if !strings.Contains(errOut.String(), "boom") {
			t.Errorf("stderr = %q, want the raw list error", errOut.String())
		}
	})
}

// TestProbeSecretBackendLookupErrors covers probeSecretBackend's remaining
// branches: a lookup that errors (after a successful store) reports the failure,
// and a session backend whose Lock fails on the way out logs it without changing
// the result.
func TestProbeSecretBackendLookupErrors(t *testing.T) {
	t.Run("lookup error is reported", func(t *testing.T) {
		backend := &fakeProbeBackend{lookupErr: errors.New("boom")}
		var buf bytes.Buffer
		if got := probeSecretBackend(&buf, fakeLogger{}, backend, "probe"); got != 1 {
			t.Fatalf("probeSecretBackend (lookup errors) = %d, want 1", got)
		}
		if !strings.Contains(buf.String(), "lookup: FAILED") {
			t.Errorf("output = %q, want a lookup failure", buf.String())
		}
	})

	t.Run("lock failure on exit is logged", func(t *testing.T) {
		backend := &fakeProbeBackend{lookupVal: "probe", lookupOK: true, lockErr: errors.New("stuck")}
		session := fakeProbeSession{backend}
		var buf bytes.Buffer
		if got := probeSecretBackend(&buf, fakeLogger{}, session, "probe"); got != 0 {
			t.Fatalf("probeSecretBackend (lock fails on exit) = %d, want 0 (probe still passed)", got)
		}
	})
}

// TestDispatchRoutesToAskpass covers dispatch's askpass branch: with the askpass
// env marker set and args that are not a subcommand, it routes to the SSH_ASKPASS
// broker rather than normal subcommand dispatch. The broker answers from a fake
// wallet, so no terminal is touched. HOME/XDG_STATE_HOME point at a temp dir.
func TestDispatchRoutesToAskpass(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_STATE_HOME", tmp)
	t.Setenv(keys.EnvPassHandoffToken, "")

	d := depsReturning(&fakeProbeBackend{lookupVal: "wallet-pass", lookupOK: true})
	prompt := "Enter passphrase for key '/home/u/.ssh/id_ed25519': "
	var out bytes.Buffer
	if got := dispatch(d, &out, io.Discard, []string{prompt}, true); got != 0 {
		t.Fatalf("dispatch(askpass) = %d, want 0", got)
	}
	if out.String() != "wallet-pass\n" {
		t.Errorf("output = %q, want the wallet passphrase", out.String())
	}
}

// TestRunHelpAndUnknown covers run's remaining terminal cases: help prints the
// usage text and succeeds, and an unknown command prints usage to stderr and
// returns the usage exit code.
func TestRunHelpAndUnknown(t *testing.T) {
	d := realDeps()

	t.Run("help prints usage", func(t *testing.T) {
		var out, errOut bytes.Buffer
		if got := d.run(&out, &errOut, []string{"help"}); got != 0 {
			t.Fatalf("run help = %d, want 0", got)
		}
		if !strings.Contains(out.String(), "usage: sshakku") {
			t.Errorf("output = %q, want the usage text", out.String())
		}
	})

	t.Run("unknown command returns 2", func(t *testing.T) {
		var out, errOut bytes.Buffer
		if got := d.run(&out, &errOut, []string{"bogus"}); got != 2 {
			t.Fatalf("run bogus = %d, want 2", got)
		}
		if !strings.Contains(errOut.String(), "unknown command") {
			t.Errorf("stderr = %q, want an unknown-command diagnostic", errOut.String())
		}
	})
}

// TestRunDispatch covers run's remaining subcommand cases end to end, so the
// dispatch wiring is exercised (not just the command bodies directly): shell-init
// and ensure-agent drive a fake ensurer to a healthy socket, and askpass-env in a
// headless session wires the broker just as it does under a desktop.
func TestRunDispatch(t *testing.T) {
	t.Run("shell-init", func(t *testing.T) {
		tempRuntimeEnv(t)
		d := depsWithEnsurer(fakeEnsurer{res: agent.EnsureResult{LiveSock: "/run/sshakku/agent.sock"}})
		if got := d.run(io.Discard, io.Discard, []string{"shell-init"}); got != 0 {
			t.Errorf("run shell-init = %d, want 0", got)
		}
	})

	t.Run("ensure-agent", func(t *testing.T) {
		tempRuntimeEnv(t)
		d := depsWithEnsurer(fakeEnsurer{res: agent.EnsureResult{LiveSock: "/run/sshakku/agent.sock"}})
		if got := d.run(io.Discard, io.Discard, []string{"ensure-agent"}); got != 0 {
			t.Errorf("run ensure-agent = %d, want 0", got)
		}
	})

	// A session with no graphical prompter must still get the exports: reading
	// the wallet needs no display, and without them ssh prompts on the terminal
	// for a passphrase the wallet already holds. The exports are the whole
	// mechanism, so asserting only the exit code would pass either way.
	t.Run("askpass-env headless", func(t *testing.T) {
		d := realDeps()
		d.guiAvailable = func() bool { return false }
		d.self = func() (string, error) { return "/opt/sshakku/bin/sshakku", nil }
		var out, errOut bytes.Buffer
		if got := d.run(&out, &errOut, []string{"askpass-env"}); got != 0 {
			t.Fatalf("run askpass-env (headless) = %d, want 0; stderr=%q", got, errOut.String())
		}
		if out.String() != askpassExports("/opt/sshakku/bin/sshakku") {
			t.Errorf("stdout = %q, want the same exports a graphical session gets", out.String())
		}
	})
}

// TestAskpassBrokerTerminal covers askpassBroker's two remaining branches through
// the injected TTY seam: a wallet hit whose reply cannot be written surfaces as a
// write error, and a wallet miss with no terminal declines the prompt.
// HOME/XDG_STATE_HOME point at a temp dir so the session log stays off the real
// state dir.
func TestAskpassBrokerTerminal(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_STATE_HOME", tmp)
	prompt := "Enter passphrase for key '/home/u/.ssh/id_ed25519': "

	t.Run("write error on a wallet hit returns 1", func(t *testing.T) {
		d := depsReturning(&fakeProbeBackend{lookupVal: "wallet-pass", lookupOK: true})
		if got := d.askpassBroker(errWriter{}, []string{prompt}); got != 1 {
			t.Errorf("askpassBroker (failing stdout) = %d, want 1", got)
		}
	})

	t.Run("wallet miss with no terminal declines", func(t *testing.T) {
		d := depsReturning(&fakeProbeBackend{lookupOK: false})
		d.tty = fakeTTY{err: keys.ErrNoTerminal}
		if got := d.askpassBroker(io.Discard, []string{prompt}); got != 1 {
			t.Errorf("askpassBroker (wallet miss, no tty) = %d, want 1", got)
		}
	})
}

// TestRandomProbeValueError covers randomProbeValue's read-failure branch, and
// testSecretBackend's propagation of it, by stubbing the probe RNG to fail — the
// one path crypto/rand never takes on its own.
func TestRandomProbeValueError(t *testing.T) {
	orig := randRead
	t.Cleanup(func() { randRead = orig })
	randRead = func([]byte) (int, error) { return 0, errors.New("no entropy") }

	t.Run("randomProbeValue reports it", func(t *testing.T) {
		if _, err := randomProbeValue(); err == nil {
			t.Error("randomProbeValue() = nil error, want the RNG failure")
		}
	})

	t.Run("testSecretBackend fails before touching the backend", func(t *testing.T) {
		tmp := t.TempDir()
		d := depsReturning(newMemoryBackend())
		var out, errOut bytes.Buffer
		if got := d.testSecretBackend(&out, &errOut, paths.Layout{ConfigDir: tmp}, fakeLogger{}, "keychain"); got != 1 {
			t.Fatalf("testSecretBackend (RNG fails) = %d, want 1", got)
		}
		if !strings.Contains(errOut.String(), "no entropy") {
			t.Errorf("stderr = %q, want the RNG failure", errOut.String())
		}
	})
}

// TestLoadKeysSeams covers loadKeys' two seam-gated branches: an executable
// lookup failure aborts with exit 1, and a GUI-available session selects the
// graphical prompter and still loads cleanly with no keys present. A temp HOME
// with no ~/.ssh keeps enumeration empty and off the real key dir.
func TestLoadKeysSeams(t *testing.T) {
	t.Run("executable lookup failure returns 1", func(t *testing.T) {
		tempRuntimeEnv(t)
		d := depsReturning(newMemoryBackend())
		d.self = func() (string, error) { return "", errors.New("no exe") }
		var errOut bytes.Buffer
		if got := d.loadKeys(&errOut); got != 1 {
			t.Fatalf("loadKeys (self fails) = %d, want 1", got)
		}
		if !strings.Contains(errOut.String(), "no exe") {
			t.Errorf("stderr = %q, want the lookup error", errOut.String())
		}
	})

	t.Run("GUI available selects the graphical prompter", func(t *testing.T) {
		tempRuntimeEnv(t)
		d := depsReturning(newMemoryBackend())
		d.guiAvailable = func() bool { return true }
		var errOut bytes.Buffer
		if got := d.loadKeys(&errOut); got != 0 {
			t.Fatalf("loadKeys (GUI, no keys) = %d, want 0; stderr=%q", got, errOut.String())
		}
	})
}
