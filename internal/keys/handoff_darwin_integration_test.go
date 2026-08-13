//go:build darwin

package keys

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAddWithAskpassRealBinaryDarwin exercises the full production path on
// Darwin: AddWithAskpass stashes the passphrase over a private Unix socket
// (handoff_darwin.go/handoff_socket.go), spawns a real detached ssh-add,
// which execs the real askpass helper as its SSH_ASKPASS program, which
// fetches the passphrase back over that same socket. Unlike this package's
// Linux equivalent (keyadd_ttl_test.go), which redeems the stash directly
// via `keyctl pipe`, there is no standalone CLI tool to bypass the sshakku
// binary with here, so this test builds and runs the real binary — the only
// way to exercise the fetch side (cmd/sshakku's askpass dispatch) for real.
func TestAddWithAskpassRealBinaryDarwin(t *testing.T) {
	requireRealSSHBinaries(t)

	dir := shortDir(t)
	askpass := buildAskpassHelper(t, dir)

	keyfile := filepath.Join(dir, "id_test")
	const passphrase = "sshakku-darwin-handoff-test-passphrase"
	out, err := exec.CommandContext(t.Context(), "ssh-keygen", "-t", "ed25519", "-N", passphrase, "-f", keyfile, "-q").CombinedOutput()
	require.NoErrorf(t, err, "a real passphrase-protected key to load:\n%s", out)

	sock := filepath.Join(dir, "agent.sock")
	agentCmd := exec.CommandContext(t.Context(), "ssh-agent", "-D", "-a", sock)
	require.NoError(t, agentCmd.Start(), "a real ssh-agent to load it into")
	t.Cleanup(func() {
		_ = agentCmd.Process.Kill()
		_ = agentCmd.Wait()
	})
	waitForSocket(t, sock)
	t.Setenv("SSH_AUTH_SOCK", sock)
	t.Setenv("HOME", shortDir(t))

	adder := ExecKeyAdder{AskpassProg: askpass}
	rc, err := adder.AddWithAskpass(t.Context(), keyfile, passphrase)
	require.NoError(t, err, "loading the key through the real handoff must succeed")
	require.Zero(t, rc, "and ssh-add must accept the passphrase it collected")

	runner := ExecRunner{}
	fp, err := FileFingerprint(t.Context(), runner, keyfile)
	require.NoError(t, err, "reading the key's fingerprint must succeed")
	loaded, err := AgentFingerprints(t.Context(), runner)
	require.NoError(t, err, "asking the agent what it holds must succeed")
	assert.Containsf(t, loaded, fp,
		"the key must be in the agent: the passphrase travelled from here to a detached ssh-add through the "+
			"real socket rendezvous and back, and nothing but the agent holding the key proves it arrived: %v", loaded)
}
