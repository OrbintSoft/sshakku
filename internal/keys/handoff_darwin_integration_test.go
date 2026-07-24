//go:build darwin

package keys

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// requireRealSSHToolsNoKeyctl is requireRealSSHTools without the keyctl
// check: keyctl is Linux-only and irrelevant here — this test exercises the
// Darwin socket handoff (handoff_darwin.go), not the keyring.
func requireRealSSHToolsNoKeyctl(t *testing.T) {
	t.Helper()
	for _, bin := range []string{"ssh-agent", "ssh-add", "ssh-keygen"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not on PATH", bin)
		}
	}
}

// TestAddWithAskpassRealBinaryDarwin exercises the full production path on
// Darwin: AddWithAskpass stashes the passphrase over a private Unix socket
// (handoff_darwin.go/handoff_socket.go), spawns a real detached ssh-add,
// which execs the real sshakku binary as its SSH_ASKPASS helper, which
// fetches the passphrase back over that same socket. Unlike this package's
// Linux equivalent (keyadd_ttl_test.go), which redeems the stash directly
// via `keyctl pipe`, there is no standalone CLI tool to bypass the sshakku
// binary with here, so this test builds and runs the real binary — the only
// way to exercise the fetch side (cmd/sshakku's askpass dispatch) for real.
func TestAddWithAskpassRealBinaryDarwin(t *testing.T) {
	requireRealSSHToolsNoKeyctl(t)

	dir := shortDir(t)
	sshakkuBin := filepath.Join(dir, "sshakku")
	build := exec.Command("go", "build", "-o", sshakkuBin, "github.com/OrbintSoft/sshakku/cmd/sshakku")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build sshakku: %v: %s", err, out)
	}

	keyfile := filepath.Join(dir, "id_test")
	const passphrase = "sshakku-darwin-handoff-test-passphrase"
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
	t.Setenv("HOME", shortDir(t))

	adder := ExecKeyAdder{AskpassProg: sshakkuBin}
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
		t.Fatalf("AgentFingerprints: %v", err)
	}
	if !loaded[fp] {
		t.Fatal("key not present in the agent after AddWithAskpass via the real sshakku binary")
	}
}
