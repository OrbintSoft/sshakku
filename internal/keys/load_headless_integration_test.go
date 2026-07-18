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
	if out, err := exec.Command("ssh-keygen", "-t", "ed25519", "-N", passphrase, "-f", keyfile, "-q").CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen: %v: %s", err, out)
	}

	sock := filepath.Join(dir, "agent.sock")
	agentCmd := exec.Command("ssh-agent", "-D", "-a", sock)
	if err := agentCmd.Start(); err != nil {
		t.Fatalf("start ssh-agent: %v", err)
	}
	t.Cleanup(func() {
		_ = agentCmd.Process.Kill()
		_ = agentCmd.Wait()
	})
	waitForSocket(t, sock)
	t.Setenv("SSH_AUTH_SOCK", sock)

	askpassScript := filepath.Join(dir, "askpass.sh")
	script := "#!/bin/sh\nexec keyctl pipe \"$" + EnvKeyctlSerial + "\"\n"
	if err := os.WriteFile(askpassScript, []byte(script), 0o755); err != nil {
		t.Fatalf("write askpass helper: %v", err)
	}

	loader := Loader{
		Keys:   fakeLister{paths: []string{keyfile}},
		Runner: ExecRunner{},
		Secret: &fakeSecret{lookupPass: passphrase, lookupFound: true},
		Prompt: &fakePrompter{err: errors.New("must not be prompted: a vault hit should never reach the prompt step")},
		Adder:  ExecKeyAdder{AskpassProg: askpassScript},
		Log:    &fakeLogger{},
	}
	if err := loader.LoadKeys(); err != nil {
		t.Fatalf("LoadKeys: %v", err)
	}

	runner := ExecRunner{}
	fp, err := FileFingerprint(runner, keyfile)
	if err != nil {
		t.Fatalf("FileFingerprint: %v", err)
	}
	loaded, err := AgentFingerprints(runner)
	if err != nil {
		t.Fatalf("AgentFingerprints: %v", err)
	}
	if !loaded[fp] {
		t.Fatal("key not present in the agent after a headless vault-hit LoadKeys")
	}
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
	if out, err := exec.Command("ssh-keygen", "-t", "ed25519", "-N", passphrase, "-f", keyfile, "-q").CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen: %v: %s", err, out)
	}

	sock := filepath.Join(dir, "agent.sock")
	agentCmd := exec.Command("ssh-agent", "-D", "-a", sock)
	if err := agentCmd.Start(); err != nil {
		t.Fatalf("start ssh-agent: %v", err)
	}
	t.Cleanup(func() {
		_ = agentCmd.Process.Kill()
		_ = agentCmd.Wait()
	})
	waitForSocket(t, sock)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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

	if ctx.Err() != nil {
		t.Fatalf("LoadKeys blocked instead of returning promptly with no controlling terminal: %v\noutput:\n%s", ctx.Err(), out)
	}
	if elapsed > 5*time.Second {
		t.Errorf("LoadKeys took %v with no controlling terminal, want near-instant; output:\n%s", elapsed, out)
	}
	if err != nil {
		t.Fatalf("helper process did not confirm the key stayed unloaded: %v\noutput:\n%s", err, out)
	}
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
	if err := loader.LoadKeys(); err != nil {
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
