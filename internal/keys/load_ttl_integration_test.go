//go:build unix

package keys

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/OrbintSoft/sshakku/internal/keyring"
	"github.com/OrbintSoft/sshakku/internal/keystate"
)

// TestLoadKeysReloadsAfterRealExpiry is the end-to-end version of the bug
// report that prompted it: a key loaded with a short lifetime must actually
// disappear from a real agent once it elapses, and — unlike the out-of-band
// re-add scenario covered elsewhere with fakes — a second LoadKeys run
// against a real agent that genuinely dropped the key must reload it and
// record a fresh keystate expiry, not leave the stale one behind.
func TestLoadKeysReloadsAfterRealExpiry(t *testing.T) {
	requireRealSSHTools(t)
	if !keyring.Available() {
		t.Skip("kernel user keyring isn't usable for a round trip in this environment (e.g. no session-keyring link — common in CI/containers without a PAM login)")
	}

	dir := t.TempDir()
	keyfile := filepath.Join(dir, "id_test")
	const passphrase = "sshakku-reload-test-passphrase"
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

	const lifetime = 2 * time.Second
	runner := ExecRunner{}
	stateDir := filepath.Join(dir, "keystate")
	state := keystate.Store{Dir: stateDir}

	loader := Loader{
		Keys:     fakeLister{paths: []string{keyfile}},
		Runner:   runner,
		Secret:   &fakeSecret{lookupPass: passphrase, lookupFound: true},
		Adder:    ExecKeyAdder{AskpassProg: askpassScript, KeyLifetime: lifetime},
		KeyState: state,
		Config:   Config{KeyLifetime: lifetime},
	}

	if err := loader.LoadKeys(); err != nil {
		t.Fatalf("first LoadKeys: %v", err)
	}

	fp, err := FileFingerprint(runner, keyfile)
	if err != nil {
		t.Fatalf("FileFingerprint: %v", err)
	}
	keyname := filepath.Base(keyfile)

	loaded, err := AgentFingerprints(runner)
	if err != nil {
		t.Fatalf("AgentFingerprints (after first load): %v", err)
	}
	if !loaded[fp] {
		t.Fatal("key not present in the agent immediately after the first LoadKeys")
	}
	rec1, ok := state.Load(keyname)
	if !ok {
		t.Fatal("keystate has no record immediately after the first LoadKeys")
	}
	firstAddedAt := rec1.AddedAt

	// Wait for the agent to actually drop the key — not just for the record's
	// computed expiry, so this catches a regression in the real add path too.
	deadline := time.Now().Add(lifetime + 5*time.Second)
	for {
		loaded, err = AgentFingerprints(runner)
		if err != nil {
			t.Fatalf("AgentFingerprints (polling for expiry): %v", err)
		}
		if !loaded[fp] {
			break
		}
		if !time.Now().Before(deadline) {
			t.Fatal("key still present in the agent well after its lifetime elapsed")
		}
		time.Sleep(200 * time.Millisecond)
	}

	// Second LoadKeys run: the loader's own fingerprint snapshot must now see
	// the key as missing (not dedup-skip it) and reload it for real.
	if err := loader.LoadKeys(); err != nil {
		t.Fatalf("second LoadKeys: %v", err)
	}

	loaded, err = AgentFingerprints(runner)
	if err != nil {
		t.Fatalf("AgentFingerprints (after second load): %v", err)
	}
	if !loaded[fp] {
		t.Fatal("key not present in the agent after the second LoadKeys reloaded it")
	}
	rec2, ok := state.Load(keyname)
	if !ok {
		t.Fatal("keystate has no record after the second LoadKeys")
	}
	if !rec2.AddedAt.After(firstAddedAt) {
		t.Errorf("keystate AddedAt = %s, want a fresher timestamp than the first load's %s", rec2.AddedAt, firstAddedAt)
	}
	if expiresAt, hasExpiry := rec2.ExpiresAt(); !hasExpiry || !expiresAt.After(time.Now()) {
		t.Errorf("keystate record after reload should expire in the future, got hasExpiry=%v expiresAt=%s", hasExpiry, expiresAt)
	}
}
