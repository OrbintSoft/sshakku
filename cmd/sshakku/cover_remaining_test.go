package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/agent"
	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys"
	"github.com/OrbintSoft/sshakku/internal/keys/handoff"
	"github.com/OrbintSoft/sshakku/internal/keys/prompt"
	"github.com/OrbintSoft/sshakku/internal/keys/wallet"
	"github.com/OrbintSoft/sshakku/internal/paths"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeTTY stands in for the /dev/tty prompter so the broker's terminal-fallback
// decisions run without a controlling terminal: it returns a canned reply, or an
// error (e.g. prompt.ErrNoTerminal) to drive the decline path.
type fakeTTY struct {
	reply string
	err   error
}

func (f fakeTTY) Prompt(string, bool) (string, error) { return f.reply, f.err }

var _ prompt.TTY = fakeTTY{}

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
		assert.Zerof(t, d.forget(t.Context(), &out, &errOut, []string{"id_rsa"}),
			"a wallet that opens and closes cleanly is swept without complaint; stderr=%q", errOut.String())
	})

	t.Run("unlock failure still sweeps", func(t *testing.T) {
		backend := &fakeProbeBackend{unlockErr: errors.New("locked")}
		d := depsReturning(fakeProbeSession{backend})
		var out, errOut bytes.Buffer
		require.Zerof(t, d.forget(t.Context(), &out, &errOut, []string{"id_rsa"}),
			"a wallet that would not open may still let the entry go; stderr=%q", errOut.String())
		assert.Contains(t, out.String(), "forgot ",
			"so the sweep must be attempted rather than abandoned at the unlock")
	})

	t.Run("lock failure on exit is tolerated", func(t *testing.T) {
		backend := &fakeProbeBackend{lockErr: errors.New("stuck")}
		d := depsReturning(fakeProbeSession{backend})
		var out, errOut bytes.Buffer
		assert.Zerof(t, d.forget(t.Context(), &out, &errOut, []string{"id_rsa"}),
			"a wallet that would not close again does not un-forget what was removed; stderr=%q", errOut.String())
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
		assert.Equal(t, 2, d.forget(t.Context(), &out, &errOut, []string{"--all", "id_rsa"}),
			"everything and one thing are different requests, and guessing between them is not on")
		assert.Contains(t, errOut.String(), "cannot be combined", "and the refusal must say why")
	})

	t.Run("--all with an unsupported List reports the hint", func(t *testing.T) {
		d := depsReturning(&fakeProbeBackend{listErr: wallet.ErrListUnsupported})
		var out, errOut bytes.Buffer
		assert.Equal(t, 1, d.forget(t.Context(), &out, &errOut, []string{"--all"}),
			"a wallet that cannot be listed cannot be swept, and must not be reported as swept")
		assert.Contains(t, errOut.String(), "native Secret Service backend",
			"and the user must be told what would let them do it")
	})

	t.Run("--all with a failing List reports the error", func(t *testing.T) {
		d := depsReturning(&fakeProbeBackend{listErr: errors.New("boom")})
		var out, errOut bytes.Buffer
		assert.Equal(t, 1, d.forget(t.Context(), &out, &errOut, []string{"--all"}),
			"a wallet that could not be read must not be reported as emptied")
		assert.Contains(t, errOut.String(), "boom", "and the reason must reach the user unaltered")
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
		assert.Equal(t, 1, probeSecretBackend(t.Context(), &buf, fakeLogger{}, backend, "probe"),
			"a wallet that errors when read has failed the probe")
		assert.Contains(t, buf.String(), "lookup: FAILED", "and the step that failed must be named")
	})

	t.Run("lock failure on exit is logged", func(t *testing.T) {
		backend := &fakeProbeBackend{lookupVal: "probe", lookupOK: true, lockErr: errors.New("stuck")}
		session := fakeProbeSession{backend}
		var buf bytes.Buffer
		assert.Zero(t, probeSecretBackend(t.Context(), &buf, fakeLogger{}, session, "probe"),
			"the round trip worked; a wallet that would not lock again does not undo that")
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
	t.Setenv(handoff.EnvToken, "")

	d := depsReturning(&fakeProbeBackend{lookupVal: "wallet-pass", lookupOK: true})
	question := "Enter passphrase for key '/home/u/.ssh/id_ed25519': "
	var out bytes.Buffer
	require.Zero(t, dispatch(t.Context(), d, &out, io.Discard, "/usr/local/bin/"+askpassProgName, []string{question}),
		"run under the helper's name, arguments are a prompt to answer")
	assert.Equal(t, "wallet-pass\n", out.String(), "answered from the wallet, not from a terminal")
}

// TestRunHelpAndUnknown covers run's remaining terminal cases: help prints the
// usage text and succeeds, and an unknown command prints usage to stderr and
// returns the usage exit code.
func TestRunHelpAndUnknown(t *testing.T) {
	d := realDeps()

	t.Run("help prints usage", func(t *testing.T) {
		var out, errOut bytes.Buffer
		require.Zero(t, d.run(t.Context(), &out, &errOut, []string{"help"}), "asking for help is not a failure")
		assert.Contains(t, out.String(), "usage: sshakku", "and help is what must be printed")
	})

	t.Run("unknown command returns 2", func(t *testing.T) {
		var out, errOut bytes.Buffer
		assert.Equal(t, 2, d.run(t.Context(), &out, &errOut, []string{"bogus"}), "a command that does not exist is a usage error")
		assert.Contains(t, errOut.String(), "unknown command", "and the user must be told that is what happened")
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
		assert.Zero(t, d.run(t.Context(), io.Discard, io.Discard, []string{"shell-init"}),
			"shell-init must reach the same healthy agent through dispatch as it does directly")
	})

	t.Run("ensure-agent", func(t *testing.T) {
		tempRuntimeEnv(t)
		d := depsWithEnsurer(fakeEnsurer{res: agent.EnsureResult{LiveSock: "/run/sshakku/agent.sock"}})
		assert.Zero(t, d.run(t.Context(), io.Discard, io.Discard, []string{"ensure-agent"}),
			"and so must ensure-agent")
	})

	// A session with no graphical prompter must still get the exports: reading
	// the wallet needs no display, and without them ssh prompts on the terminal
	// for a passphrase the wallet already holds. The exports are the whole
	// mechanism, so asserting only the exit code would pass either way.
	t.Run("askpass-env headless", func(t *testing.T) {
		d := realDeps()
		d.graphicalPrompter = func(context.Context, config.Settings, keys.Logger) prompt.Prompter { return nil }
		d.self = func() (string, error) { return "/opt/sshakku/bin/sshakku", nil }
		var out, errOut bytes.Buffer
		require.Zerof(t, d.run(t.Context(), &out, &errOut, []string{"askpass-env"}),
			"a session with no dialog is still one the broker serves; stderr=%q", errOut.String())
		assert.Equal(t, askpassExports(dialect(t, shellPosix), "/opt/sshakku/bin/sshakku"), out.String(),
			"and it gets the same exports a graphical session does")
	})
}

// TestAskpassBrokerTerminal covers askpassBroker's two remaining branches through
// the injected prompt.TTY seam: a wallet hit whose reply cannot be written surfaces as a
// write error, and a wallet miss with no terminal declines the prompt.
// HOME/XDG_STATE_HOME point at a temp dir so the session log stays off the real
// state dir.
func TestAskpassBrokerTerminal(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_STATE_HOME", tmp)
	question := "Enter passphrase for key '/home/u/.ssh/id_ed25519': "

	t.Run("write error on a wallet hit returns 1", func(t *testing.T) {
		d := depsReturning(&fakeProbeBackend{lookupVal: "wallet-pass", lookupOK: true})
		assert.Equal(t, 1, d.askpassBroker(t.Context(), errWriter{}, []string{question}),
			"a passphrase ssh never received must not be reported as given")
	})

	t.Run("wallet miss with no terminal declines", func(t *testing.T) {
		d := depsReturning(&fakeProbeBackend{lookupOK: false})
		d.tty = fakeTTY{err: prompt.ErrNoTerminal}
		assert.Equal(t, 1, d.askpassBroker(t.Context(), io.Discard, []string{question}),
			"with nothing in the wallet and nowhere to ask, the prompt is declined rather than answered blank")
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
		_, err := randomProbeValue()
		assert.Error(t, err, "a probe value that is not random is not a probe value")
	})

	t.Run("testSecretBackend fails before touching the backend", func(t *testing.T) {
		tmp := t.TempDir()
		d := depsReturning(newMemoryBackend())
		var out, errOut bytes.Buffer
		assert.Equal(t, 1, d.testSecretBackend(t.Context(), &out, &errOut, paths.Layout{ConfigDir: tmp}, fakeLogger{}, "keychain"),
			"a probe that could not be composed must not be reported as a wallet that failed")
		assert.Contains(t, errOut.String(), "no entropy", "and the real reason must reach the user")
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
		assert.Equal(t, 1, d.loadKeys(t.Context(), &errOut),
			"without a path to itself there is no askpass to hand ssh, and loading must not pretend otherwise")
		assert.Contains(t, errOut.String(), "no exe", "and the reason must reach the user")
	})

	t.Run("a platform with a dialog selects it over the terminal", func(t *testing.T) {
		tempRuntimeEnv(t)
		d := depsReturning(newMemoryBackend())
		d.graphicalPrompter = func(context.Context, config.Settings, keys.Logger) prompt.Prompter {
			return fixedPrompter{answer: "from the dialog"}
		}
		var errOut bytes.Buffer
		assert.Zerof(t, d.loadKeys(t.Context(), &errOut),
			"a session with a dialog loads no differently from one without; stderr=%q", errOut.String())
	})
}
