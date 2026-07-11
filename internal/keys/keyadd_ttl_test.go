//go:build unix

package keys

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/OrbintSoft/sshakku/internal/keyring"
)

// requireRealSSHTools skips the test when the real ssh-agent/ssh-add/ssh-keygen
// and keyctl binaries this test drives aren't on PATH.
func requireRealSSHTools(t *testing.T) {
	t.Helper()
	for _, bin := range []string{"ssh-agent", "ssh-add", "ssh-keygen", "keyctl"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not on PATH", bin)
		}
	}
}

// TestAddWithAskpassAppliesKeyLifetime exercises the exact code path a GUI
// session uses to load a key (a detached, setsid ssh-add fed through the
// SSH_ASKPASS + keyring handoff) against a real ssh-agent, and checks that
// the agent actually drops the key once the requested lifetime elapses — not
// just that ssh-add accepts the -t flag. This is the one link in the chain
// AddWithAskpass's unit tests (which stub the agent) don't cover.
func TestAddWithAskpassAppliesKeyLifetime(t *testing.T) {
	requireRealSSHTools(t)
	if !keyring.Available() {
		t.Skip("kernel user keyring isn't usable for a round trip in this environment (e.g. no session-keyring link — common in CI/containers without a PAM login)")
	}

	dir := t.TempDir()
	keyfile := filepath.Join(dir, "id_test")
	const passphrase = "sshakku-ttl-test-passphrase"

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

	// A minimal askpass helper mirroring askpassFromKeyring in cmd/sshakku:
	// print the payload AddWithAskpass stashed under $SSHAKKU_KEYCTL_SERIAL.
	askpassScript := filepath.Join(dir, "askpass.sh")
	script := "#!/bin/sh\nexec keyctl pipe \"$" + EnvKeyctlSerial + "\"\n"
	if err := os.WriteFile(askpassScript, []byte(script), 0o755); err != nil {
		t.Fatalf("write askpass helper: %v", err)
	}

	const lifetime = 2 * time.Second
	adder := ExecKeyAdder{AskpassProg: askpassScript, KeyLifetime: lifetime}
	rc, err := adder.AddWithAskpass(keyfile, passphrase)
	if err != nil {
		t.Fatalf("AddWithAskpass: %v", err)
	}
	if rc != 0 {
		t.Fatalf("AddWithAskpass: ssh-add exited %d", rc)
	}

	runner := ExecRunner{}
	fp, err := FileFingerprint(runner, keyfile)
	if err != nil {
		t.Fatalf("FileFingerprint: %v", err)
	}

	loaded, err := AgentFingerprints(runner)
	if err != nil {
		t.Fatalf("AgentFingerprints (immediately after add): %v", err)
	}
	if !loaded[fp] {
		t.Fatal("key not present in the agent immediately after AddWithAskpass")
	}

	deadline := time.Now().Add(lifetime + 5*time.Second)
	for time.Now().Before(deadline) {
		loaded, err = AgentFingerprints(runner)
		if err != nil {
			t.Fatalf("AgentFingerprints (polling for expiry): %v", err)
		}
		if !loaded[fp] {
			return // expired as expected
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("key still present in the agent %s after AddWithAskpass with KeyLifetime=%s — the agent never expired it", time.Since(deadline.Add(-lifetime-5*time.Second)), lifetime)
}

// waitForSocket polls until path exists or t fails.
func waitForSocket(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("ssh-agent never created %s", path)
}
