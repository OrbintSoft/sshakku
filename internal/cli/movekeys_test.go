package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/keys/move"
)

// Verifies F70 through the command a user types. Every run goes through the
// dispatcher, and what the assertions read is the filesystem and the
// configuration afterwards — not what the command said it did.

// runMoveKeys runs the command through the dispatcher and hands back what the
// user would see.
func runMoveKeys(t *testing.T, d deps, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = d.run(t.Context(), &out, &errOut, append([]string{"move-keys"}, args...))
	return code, out.String(), errOut.String()
}

// there says whether a path is on the filesystem, which is what these tests
// assert on: a key that moved is one that is in the new place and not the old.
func there(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Lstat(path)
	return err == nil
}

// TestTheKeysEndUpInTheNewDirectoryAndTheConfigurationSaysSo is F70 whole: the
// keys move, their public halves go with them, and the same keys go on being
// loaded because the user's own configuration now names where they are.
func TestTheKeysEndUpInTheNewDirectoryAndTheConfigurationSaysSo(t *testing.T) {
	home := tempRuntimeEnv(t)
	old := filepath.Join(home, ".ssh")
	require.NoError(t, os.MkdirAll(old, 0o700))
	for _, name := range []string{"id_ed25519", "id_ed25519.pub"} {
		require.NoError(t, os.WriteFile(filepath.Join(old, name), []byte("not really a key"), 0o600))
	}
	newDir := filepath.Join(home, "keys")

	code, out, errOut := runMoveKeys(t, realDeps(), newDir)

	require.Zerof(t, code, "the move succeeds; stderr=%q", errOut)
	assert.True(t, there(t, filepath.Join(newDir, "id_ed25519")), "the key is in the new directory")
	assert.True(t, there(t, filepath.Join(newDir, "id_ed25519.pub")), "and so is its public half")
	assert.False(t, there(t, filepath.Join(old, "id_ed25519")),
		"and not in the old one — it moved, it was not copied")
	assert.Contains(t, out, newDir, "the run says where they went")

	var config, configErr bytes.Buffer
	require.Zero(t, realDeps().run(t.Context(), &config, &configErr, []string{"config"}))
	assert.Contains(t, config.String(), newDir, "and the configuration names it as the key directory")
}

// TestAuthorizedKeysStaysWhereItIs. That file belongs to the SSH server rather
// than to the user, and moving it is what stops logins into this machine.
func TestAuthorizedKeysStaysWhereItIs(t *testing.T) {
	home := tempRuntimeEnv(t)
	old := filepath.Join(home, ".ssh")
	require.NoError(t, os.MkdirAll(old, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(old, "id_ed25519"), []byte("key"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(old, "authorized_keys"), []byte("ssh-ed25519 AAAA"), 0o600))

	code, _, errOut := runMoveKeys(t, realDeps(), filepath.Join(home, "keys"))

	require.Zerof(t, code, "the move succeeds; stderr=%q", errOut)
	assert.True(t, there(t, filepath.Join(old, "authorized_keys")), "it is still where the server reads it")
	assert.False(t, there(t, filepath.Join(home, "keys", "authorized_keys")), "and was not taken along")
}

// TestANameAlreadyTakenStopsEverything. A move onto an existing file overwrites
// it, and the file it would overwrite is somebody's key.
func TestANameAlreadyTakenStopsEverything(t *testing.T) {
	home := tempRuntimeEnv(t)
	old := filepath.Join(home, ".ssh")
	newDir := filepath.Join(home, "keys")
	require.NoError(t, os.MkdirAll(old, 0o700))
	require.NoError(t, os.MkdirAll(newDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(old, "id_first"), []byte("key"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(old, "id_second"), []byte("key"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(newDir, "id_second"), []byte("somebody else's"), 0o600))

	code, _, errOut := runMoveKeys(t, realDeps(), newDir)

	assert.NotZero(t, code, "nothing was moved, and the exit code says so")
	assert.Contains(t, errOut, "id_second", "what stopped it is named")
	assert.True(t, there(t, filepath.Join(old, "id_first")),
		"and the key that could have moved did not: nothing is done by halves")
	assert.True(t, there(t, filepath.Join(old, "id_second")))

	kept, err := os.ReadFile(filepath.Join(newDir, "id_second"))
	require.NoError(t, err)
	assert.Equal(t, "somebody else's", string(kept), "the file already there is untouched")
}

// TestADropInThatDecidesWhereTheKeysLiveIsARefusal. Writing config.toml is how
// the move is made to stick; a drop-in read afterwards would overrule it, and
// every login would go on looking in the old place with nothing to say why.
func TestADropInThatDecidesWhereTheKeysLiveIsARefusal(t *testing.T) {
	home := tempRuntimeEnv(t)
	old := filepath.Join(home, ".ssh")
	require.NoError(t, os.MkdirAll(old, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(old, "id_ed25519"), []byte("key"), 0o600))
	writeConfig(t, home, filepath.Join("config.d", "50-work.toml"), "key_dir = \".ssh\"\n")

	code, _, errOut := runMoveKeys(t, realDeps(), filepath.Join(home, "keys"))

	assert.NotZero(t, code)
	assert.Contains(t, errOut, "50-work.toml", "the file that would have overruled it is named")
	assert.True(t, there(t, filepath.Join(old, "id_ed25519")), "and nothing moved")
}

// TestADirectoryThatCannotBeGivenTheRightPermissionsStopsEverything. A key
// sitting in a directory anyone can enter is a key OpenSSH refuses to use, so a
// move that could not be finished is not a move worth making.
func TestADirectoryThatCannotBeGivenTheRightPermissionsStopsEverything(t *testing.T) {
	home := tempRuntimeEnv(t)
	old := filepath.Join(home, ".ssh")
	require.NoError(t, os.MkdirAll(old, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(old, "id_ed25519"), []byte("key"), 0o600))

	d := realDeps()
	d.permitKey = func(string, move.Kind) error { return assert.AnError }

	code, _, errOut := runMoveKeys(t, d, filepath.Join(home, "keys"))

	assert.NotZero(t, code)
	assert.Contains(t, errOut, "permission", "what it could not do is named in the user's terms")
	assert.True(t, there(t, filepath.Join(old, "id_ed25519")), "and the key is where it was")
}

// TestMovingKeysWhereThereAreNoneChangesNothing, rather than writing a
// configuration for keys that do not exist.
func TestMovingKeysWhereThereAreNoneChangesNothing(t *testing.T) {
	home := tempRuntimeEnv(t)
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".ssh"), 0o700))

	code, out, errOut := runMoveKeys(t, realDeps(), filepath.Join(home, "keys"))

	require.Zerof(t, code, "having nothing to do is not a failure; stderr=%q", errOut)
	assert.Contains(t, out, "No keys")
	assert.False(t, there(t, filepath.Join(home, ".config", "sshakku", "config.toml")),
		"and no configuration is written for a move that did not happen")
}

// TestMovingTheKeysWhereTheyAlreadyAreIsARefusal, since the alternative is a
// run that moves every file onto itself and reports success.
func TestMovingTheKeysWhereTheyAlreadyAreIsARefusal(t *testing.T) {
	home := tempRuntimeEnv(t)
	old := filepath.Join(home, ".ssh")
	require.NoError(t, os.MkdirAll(old, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(old, "id_ed25519"), []byte("key"), 0o600))

	code, _, errOut := runMoveKeys(t, realDeps(), old)

	assert.NotZero(t, code)
	assert.Contains(t, errOut, "already", "and it says so rather than doing nothing quietly")
	assert.True(t, there(t, filepath.Join(old, "id_ed25519")))
}

func TestMoveKeysNeedsADirectoryToMoveThemTo(t *testing.T) {
	tempRuntimeEnv(t)

	code, _, errOut := runMoveKeys(t, realDeps())

	assert.Equal(t, 2, code, "a usage error, as every other command answers one")
	assert.Contains(t, errOut, "directory", "what is missing is named")
	assert.NotContains(t, errOut, "unknown command", "the command is known; the argument is what is absent")
}

func TestMoveKeysRefusesAnArgumentItDoesNotKnow(t *testing.T) {
	tempRuntimeEnv(t)

	code, _, errOut := runMoveKeys(t, realDeps(), "--everything", filepath.Join(t.TempDir(), "keys"))

	assert.Equal(t, 2, code)
	assert.Contains(t, errOut, "--everything")
}
