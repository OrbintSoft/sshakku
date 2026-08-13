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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	require.NoError(t, loader.LoadKeys(t.Context()), "the first login of the day must load the key")

	fp, err := FileFingerprint(runner, keyfile)
	require.NoError(t, err, "reading the key's fingerprint must succeed")
	keyname := filepath.Base(keyfile)

	loaded, err := AgentFingerprints(runner)
	require.NoError(t, err, "asking the agent what it holds must succeed")
	require.Containsf(t, loaded, fp, "and the key must be in it: %v", loaded)
	rec1, ok := state.Load(keyname)
	require.True(t, ok, "with a record of when it was added, or nothing knows when it will be gone")
	firstAddedAt := rec1.AddedAt

	// Wait for the agent to actually drop the key — not just for the record's
	// computed expiry, so this catches a regression in the real add path too.
	deadline := time.Now().Add(lifetime + 5*time.Second)
	for {
		loaded, err = AgentFingerprints(runner)
		require.NoError(t, err, "asking the agent what it holds must keep succeeding")
		if !loaded[fp] {
			break
		}
		require.Truef(t, time.Now().Before(deadline),
			"the agent never expired the key: it is still held well past the %s lifetime it was added with", lifetime)
		time.Sleep(200 * time.Millisecond)
	}

	// Second LoadKeys run: the loader's own fingerprint snapshot must now see
	// the key as missing (not dedup-skip it) and reload it for real.
	require.NoError(t, loader.LoadKeys(t.Context()), "a later login must load the key again")

	loaded, err = AgentFingerprints(runner)
	require.NoError(t, err, "asking the agent what it holds must succeed")
	require.Containsf(t, loaded, fp,
		"the key must be back: an agent that dropped it is an agent whose snapshot no longer names it, "+
			"and skipping it there is how a key silently stops working mid-day: %v", loaded)
	rec2, ok := state.Load(keyname)
	require.True(t, ok, "with a record of the reload")
	assert.Truef(t, rec2.AddedAt.After(firstAddedAt),
		"stamped now rather than left at the first load's %s, or the doctor reports a key as expired while it works",
		firstAddedAt)
	expiresAt, hasExpiry := rec2.ExpiresAt()
	require.True(t, hasExpiry, "a key added with a lifetime has an expiry")
	assert.True(t, expiresAt.After(time.Now()), "and this one is ahead, not the one that already went by")
}
