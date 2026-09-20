package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/keys/move"
	"github.com/OrbintSoft/sshakku/internal/sessionlog"
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

// TestAConfigurationThatCouldNotBeWrittenPutsTheKeysBack is the half of F70
// that costs the most to get wrong and is the hardest to notice: the keys are
// in the new directory, nothing says so, and every login afterwards looks where
// they no longer are. The run puts them back rather than leaving that behind.
//
// The failure is arranged by making config.toml a directory. The check made
// before anything moves asks whether the configuration directory can be written
// and it still can — a file is made beside the target and removed again — so
// the refusal happens at the write itself, which is the ordering this needs.
func TestAConfigurationThatCouldNotBeWrittenPutsTheKeysBack(t *testing.T) {
	home := tempRuntimeEnv(t)
	old := filepath.Join(home, ".ssh")
	require.NoError(t, os.MkdirAll(old, 0o700))
	for _, name := range []string{"id_ed25519", "id_ed25519.pub"} {
		require.NoError(t, os.WriteFile(filepath.Join(old, name), []byte("not really a key"), 0o600))
	}
	require.NoError(t, os.MkdirAll(filepath.Join(home, ".config", "sshakku", "config.toml"), 0o700))
	newDir := filepath.Join(home, "keys")

	code, _, errOut := runMoveKeys(t, realDeps(), newDir)

	assert.NotZero(t, code, "a move nothing records is a failure, not a success with a note")
	assert.True(t, there(t, filepath.Join(old, "id_ed25519")), "the key is back where it came from")
	assert.True(t, there(t, filepath.Join(old, "id_ed25519.pub")), "and so is its public half")
	assert.False(t, there(t, filepath.Join(newDir, "id_ed25519")),
		"and not in the directory the configuration was never told about")
	assert.Contains(t, errOut, "your keys are where they were",
		"the user is told the state they are actually in")
}

// TestWhatCouldNotBePutBackIsNamed covers the outcome no rollback can rescue:
// files moved out and not moved back. Naming them is the whole of what is left
// to do for the user, so the list is the behaviour rather than a detail of it.
//
// It is driven directly because the rename this would need to fail is the
// filesystem's own, and there is no arrangement of a directory that makes
// putting a file back where it just came from fail on every system this runs on.
func TestWhatCouldNotBePutBackIsNamed(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "sessions.log")
	var errOut bytes.Buffer

	moveKeysStopped(&errOut, sessionlog.New(logFile), assert.AnError,
		move.Result{Stranded: []string{"/keys/id_ed25519", "/keys/id_ed25519.pub"}})

	assert.Contains(t, errOut.String(), "/keys/id_ed25519", "the file left behind is named")
	assert.Contains(t, errOut.String(), "/keys/id_ed25519.pub", "and so is every other one")
	assert.NotContains(t, errOut.String(), "your keys are where they were",
		"which they are not, and saying so would be the one thing worse than the failure")

	recorded, err := os.ReadFile(logFile)
	require.NoError(t, err, "the session log is where this is worked back from later")
	assert.Contains(t, string(recorded), "/keys/id_ed25519")
}

// TestAKeyDirectoryThatCannotBeMadeStopsEverything, before a key moves into a
// directory that is not there.
func TestAKeyDirectoryThatCannotBeMadeStopsEverything(t *testing.T) {
	home := tempRuntimeEnv(t)
	old := filepath.Join(home, ".ssh")
	require.NoError(t, os.MkdirAll(old, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(old, "id_ed25519"), []byte("key"), 0o600))
	// A file, so nothing can be made underneath it.
	inTheWay := filepath.Join(home, "not-a-directory")
	require.NoError(t, os.WriteFile(inTheWay, nil, 0o600))

	code, _, errOut := runMoveKeys(t, realDeps(), filepath.Join(inTheWay, "keys"))

	assert.NotZero(t, code)
	assert.Contains(t, errOut, inTheWay, "the path it could not make is named")
	assert.True(t, there(t, filepath.Join(old, "id_ed25519")), "and the key has not moved")
}

// TestAConfigurationThatCannotBeWrittenIsARefusal, found before anything moves
// rather than after: a refusal discovered later is a user with keys in two
// places.
func TestAConfigurationThatCannotBeWrittenIsARefusal(t *testing.T) {
	home := tempRuntimeEnv(t)
	old := filepath.Join(home, ".ssh")
	require.NoError(t, os.MkdirAll(old, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(old, "id_ed25519"), []byte("key"), 0o600))
	// A file where the configuration directory's parent goes, so the directory
	// itself can never be made.
	require.NoError(t, os.WriteFile(filepath.Join(home, ".config"), nil, 0o600))
	newDir := filepath.Join(home, "keys")

	code, _, errOut := runMoveKeys(t, realDeps(), newDir)

	assert.NotZero(t, code)
	assert.NotEmpty(t, errOut, "what stopped it is said")
	assert.True(t, there(t, filepath.Join(old, "id_ed25519")), "the key has not moved")
	assert.False(t, there(t, filepath.Join(newDir, "id_ed25519")), "and nothing is in the new directory")
}

// TestAKeyDirectoryThatIsNotADirectoryIsReported rather than read as an account
// with no keys, which is what a missing one means and is a different answer.
func TestAKeyDirectoryThatIsNotADirectoryIsReported(t *testing.T) {
	home := tempRuntimeEnv(t)
	require.NoError(t, os.WriteFile(filepath.Join(home, ".ssh"), nil, 0o600))

	code, out, errOut := runMoveKeys(t, realDeps(), filepath.Join(home, "keys"))

	assert.NotZero(t, code)
	assert.NotContains(t, out, "No keys", "an unreadable directory is not an empty one")
	assert.Contains(t, errOut, ".ssh", "the directory it could not read is named")
}

// TestAKeyThatCannotBeGivenTheRightPermissionsIsPutBack. The directory is made
// and permitted first, so this is the failure that happens with keys already
// moved: they go back, and the run reports what it could not do.
func TestAKeyThatCannotBeGivenTheRightPermissionsIsPutBack(t *testing.T) {
	home := tempRuntimeEnv(t)
	old := filepath.Join(home, ".ssh")
	require.NoError(t, os.MkdirAll(old, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(old, "id_ed25519"), []byte("key"), 0o600))
	newDir := filepath.Join(home, "keys")

	d := realDeps()
	// The directory is allowed, a key is not: the arrangement that gets a file
	// moved before anything refuses.
	d.permitKey = func(_ string, kind move.Kind) error {
		if kind == move.Directory {
			return nil
		}
		return assert.AnError
	}

	code, _, errOut := runMoveKeys(t, d, newDir)

	assert.NotZero(t, code)
	assert.True(t, there(t, filepath.Join(old, "id_ed25519")), "the key is back where it came from")
	assert.False(t, there(t, filepath.Join(newDir, "id_ed25519")), "and not in the new directory")
	assert.Contains(t, errOut, "your keys are where they were", "which is what the user is told")
}

// TestADirectoryNamedFromNowhereIsRefused covers the one way naming a
// directory can fail before it is even looked at: a relative path is made
// absolute against the directory the user is standing in, and a session whose
// own directory has been taken away from underneath it has nothing to resolve
// against.
func TestADirectoryNamedFromNowhereIsRefused(t *testing.T) {
	tempRuntimeEnv(t)
	gone := t.TempDir()
	t.Chdir(gone)
	require.NoError(t, os.Remove(gone), "the session's own directory is taken away")

	code, _, errOut := runMoveKeys(t, realDeps(), "keys")

	assert.Equal(t, 2, code, "a usage answer: nothing was wrong with the command, the path could not be read")
	assert.NotEmpty(t, errOut, "and what went wrong is said rather than left blank")
}
