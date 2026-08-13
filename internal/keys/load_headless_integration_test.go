//go:build unix

package keys

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/OrbintSoft/sshakku/internal/keyring"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoadKeysHeadlessVaultHit confirms the full proactive path — a real
// ssh-agent, real ssh-add, and the real keyring+SSH_ASKPASS handoff — loads a
// key from a stored passphrase with no graphical prompter involved at all: a
// Prompter that fails the test if it is ever called proves the vault is
// genuinely consulted headless, never skipped in favour of prompting just
// because no GUI is available.
func TestLoadKeysHeadlessVaultHit(t *testing.T) {
	requireRealSSHTools(t)
	if !keyring.Available() {
		t.Skip("kernel user keyring isn't usable for a round trip in this environment (e.g. no session-keyring link — common in CI/containers without a PAM login)")
	}

	dir := t.TempDir()
	keyfile := filepath.Join(dir, "id_test")
	const passphrase = "sshakku-headless-vault-test-passphrase"
	out, err := exec.Command("ssh-keygen", "-t", "ed25519", "-N", passphrase, "-f", keyfile, "-q").CombinedOutput()
	require.NoErrorf(t, err, "a real passphrase-protected key to load:\n%s", out)

	sock := filepath.Join(dir, "agent.sock")
	agentCmd := exec.Command("ssh-agent", "-D", "-a", sock)
	require.NoError(t, agentCmd.Start(), "a real ssh-agent to load it into")
	t.Cleanup(func() {
		_ = agentCmd.Process.Kill()
		_ = agentCmd.Wait()
	})
	waitForSocket(t, sock)
	t.Setenv("SSH_AUTH_SOCK", sock)

	askpassScript := filepath.Join(dir, "askpass.sh")
	script := "#!/bin/sh\nexec keyctl pipe \"$" + EnvPassHandoffToken + "\"\n"
	require.NoError(t, os.WriteFile(askpassScript, []byte(script), 0o755), "a helper to collect the stashed passphrase")

	loader := Loader{
		Keys:   fakeLister{paths: []string{keyfile}},
		Runner: ExecRunner{},
		Secret: &fakeSecret{lookupPass: passphrase, lookupFound: true},
		Prompt: &fakePrompter{err: errors.New("must not be prompted: a vault hit should never reach the prompt step")},
		Adder:  ExecKeyAdder{AskpassProg: askpassScript},
		Log:    &fakeLogger{},
	}
	require.NoError(t, loader.LoadKeys(t.Context()), "a login with a stored passphrase must load the key")

	runner := ExecRunner{}
	fp, err := FileFingerprint(runner, keyfile)
	require.NoError(t, err, "reading the key's fingerprint must succeed")
	loaded, err := AgentFingerprints(runner)
	require.NoError(t, err, "asking the agent what it holds must succeed")
	assert.Containsf(t, loaded, fp,
		"the key must be in the agent, and it got there with no dialog anywhere: a session with no screen "+
			"still has a wallet, and the prompter here fails the test if it is reached at all: %v", loaded)
}

// TestLoadKeysNoTerminalReturnsPromptly confirms that with no stored
// passphrase and no controlling terminal at all, the full proactive path —
// through the real TTYPrompter, not a fake — returns promptly rather than
// hanging, leaving the key simply unloaded. It re-execs this test binary
// detached (Setsid) into its own session against a real ssh-agent, so the
// child genuinely has no controlling terminal regardless of what the test
// runner itself has.
func TestLoadKeysNoTerminalReturnsPromptly(t *testing.T) {
	requireRealSSHTools(t)

	if os.Getenv("SSHAKKU_LOADKEYS_HELPER") == "1" {
		runLoadKeysNoTerminalHelper()
	}

	dir := t.TempDir()
	keyfile := filepath.Join(dir, "id_test")
	const passphrase = "sshakku-no-terminal-test-passphrase"
	genOut, err := exec.Command("ssh-keygen", "-t", "ed25519", "-N", passphrase, "-f", keyfile, "-q").CombinedOutput()
	require.NoErrorf(t, err, "a real passphrase-protected key to load:\n%s", genOut)

	sock := filepath.Join(dir, "agent.sock")
	agentCmd := exec.Command("ssh-agent", "-D", "-a", sock)
	require.NoError(t, agentCmd.Start(), "a real ssh-agent to load it into")
	t.Cleanup(func() {
		_ = agentCmd.Process.Kill()
		_ = agentCmd.Wait()
	})
	waitForSocket(t, sock)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestLoadKeysNoTerminalReturnsPromptly$")
	cmd.Env = append(os.Environ(),
		"SSHAKKU_LOADKEYS_HELPER=1",
		"SSH_AUTH_SOCK="+sock,
		"SSHAKKU_TEST_KEYFILE="+keyfile,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdin = nil

	start := time.Now()
	out, err := cmd.CombinedOutput()
	elapsed := time.Since(start)

	require.NoErrorf(t, ctx.Err(),
		"a login with nowhere to ask must come back at once; blocking here is a shell that never returns:\n%s", out)
	assert.Lessf(t, elapsed, 5*time.Second,
		"and it must come back at once rather than waiting anything out:\n%s", out)
	require.NoErrorf(t, err,
		"with no stored passphrase and no terminal, the key must simply stay unloaded:\n%s", out)
}

// runLoadKeysNoTerminalHelper is the detached child of
// TestLoadKeysNoTerminalReturnsPromptly: it drives a real Loader — real
// ssh-agent (named by $SSH_AUTH_SOCK), real TTYPrompter, no stored
// passphrase — and exits with a distinct code per failure mode, since this
// runs detached from go test's own reporting.
func runLoadKeysNoTerminalHelper() {
	keyfile := os.Getenv("SSHAKKU_TEST_KEYFILE")
	loader := Loader{
		Keys:   fakeLister{paths: []string{keyfile}},
		Runner: ExecRunner{},
		Secret: &fakeSecret{lookupFound: false},
		Prompt: TTYPrompter{},
		Adder:  ExecKeyAdder{},
		Log:    &fakeLogger{},
	}
	// This runs in the re-executed child, which is a process of its own with
	// no test to belong to: the root context is created here because here is
	// where the program starts.
	if err := loader.LoadKeys(context.Background()); err != nil {
		os.Exit(1)
	}

	runner := ExecRunner{}
	fp, err := FileFingerprint(runner, keyfile)
	if err != nil {
		os.Exit(2)
	}
	loaded, err := AgentFingerprints(runner)
	if err != nil {
		os.Exit(3)
	}
	if loaded[fp] {
		// Must not have loaded: there was no stored passphrase and no
		// terminal to prompt on.
		os.Exit(4)
	}
	os.Exit(0)
}
