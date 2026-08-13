//go:build unix

package keys

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/OrbintSoft/sshakku/internal/keyring"
	"github.com/stretchr/testify/require"
)

// requireRealSSHBinaries skips the test when the real ssh-agent/ssh-add/
// ssh-keygen binaries it drives aren't on PATH.
func requireRealSSHBinaries(t *testing.T) {
	t.Helper()
	for _, bin := range []string{"ssh-agent", "ssh-add", "ssh-keygen"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not on PATH", bin)
		}
	}
}

// requireRealSSHTools additionally requires keyctl, which tests that redeem the
// stashed passphrase themselves — rather than through the sshakku binary — use
// to read it back out of the kernel keyring. It is Linux-only, so tests calling
// this skip on every other platform.
func requireRealSSHTools(t *testing.T) {
	t.Helper()
	requireRealSSHBinaries(t)
	if _, err := exec.LookPath("keyctl"); err != nil {
		t.Skip("keyctl not on PATH")
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

	// A minimal askpass helper mirroring askpassFromHandoff in cmd/sshakku:
	// print the payload AddWithAskpass stashed under $SSHAKKU_HANDOFF_TOKEN.
	askpassScript := filepath.Join(dir, "askpass.sh")
	script := "#!/bin/sh\nexec keyctl pipe \"$" + EnvPassHandoffToken + "\"\n"
	require.NoError(t, os.WriteFile(askpassScript, []byte(script), 0o755), "a helper to collect the stashed passphrase")

	const lifetime = 2 * time.Second
	adder := ExecKeyAdder{AskpassProg: askpassScript, KeyLifetime: lifetime}
	rc, err := adder.AddWithAskpass(t.Context(), keyfile, passphrase)
	require.NoError(t, err, "loading the key through the real handoff must succeed")
	require.Zero(t, rc, "and ssh-add must accept the passphrase it collected")

	runner := ExecRunner{}
	fp, err := FileFingerprint(t.Context(), runner, keyfile)
	require.NoError(t, err, "reading the key's fingerprint must succeed")

	loaded, err := AgentFingerprints(t.Context(), runner)
	require.NoError(t, err, "asking the agent what it holds must succeed")
	require.Containsf(t, loaded, fp, "the key must be in the agent before there is any expiry to wait for: %v", loaded)

	deadline := time.Now().Add(lifetime + 5*time.Second)
	for time.Now().Before(deadline) {
		loaded, err = AgentFingerprints(t.Context(), runner)
		require.NoError(t, err, "asking the agent what it holds must keep succeeding")
		if !loaded[fp] {
			return // expired as expected
		}
		time.Sleep(200 * time.Millisecond)
	}
	require.FailNowf(t, "the agent never expired the key",
		"it is still held well past the %s lifetime it was added with, so a passphrase the user typed once "+
			"stays usable for as long as the agent lives", lifetime)
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
	require.FailNowf(t, "ssh-agent never came up", "it created no socket at %s", path)
}
