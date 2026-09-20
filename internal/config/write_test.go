package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Verifies the half of F70 that makes a move stick: the keys are found at the
// next login because the user's own configuration now says where they are.

func TestKeyDirIsAppendedWhereTheFileSetsNone(t *testing.T) {
	body := WithKeyDir("key_lifetime = \"2h\"\n", "/home/alice/keys")

	assert.Contains(t, body, "key_lifetime = \"2h\"", "what was there is still there")
	assert.Contains(t, body, `key_dir = "/home/alice/keys"`)
}

func TestKeyDirReplacesTheOneTheFileAlreadySets(t *testing.T) {
	body := WithKeyDir("key_dir = \"/old\"\nmax_attempts = 3\n", "/new")

	assert.Contains(t, body, `key_dir = "/new"`)
	assert.NotContains(t, body, "/old", "the old directory is gone, not left below the new one")
	assert.Contains(t, body, "max_attempts = 3", "and the rest of the file is untouched")
	assert.Equal(t, 1, strings.Count(body, "key_dir"), "exactly one assignment, whatever was there before")
}

// TestACommentedExampleIsNotAnAssignment. The template ships every setting as a
// commented example. Replacing one would leave the file explaining a value it
// does not set, while the value that is set went somewhere else entirely.
func TestACommentedExampleIsNotAnAssignment(t *testing.T) {
	body := WithKeyDir("# key_dir = \".ssh\"\n", "/home/alice/keys")

	assert.Contains(t, body, `# key_dir = ".ssh"`, "the example is left as an example")
	assert.Contains(t, body, `key_dir = "/home/alice/keys"`, "and the setting is added")
}

// TestAWindowsPathSurvivesBeingWrittenAndReadBack is the one that would bite in
// practice: a backslash starts an escape inside a TOML string, so a directory
// pasted in unquoted comes back as something else or not at all.
func TestAWindowsPathSurvivesBeingWrittenAndReadBack(t *testing.T) {
	dir := `D:\keys\ssh`

	configDir := t.TempDir()
	require.NoError(t, SetKeyDir(configDir, dir))

	file, err := Load(MainFile(configDir))

	require.NoError(t, err, "what was written must parse")
	require.NotNil(t, file.KeyDir)
	assert.Equal(t, dir, *file.KeyDir, "and read back as the same directory")
}

func TestSetKeyDirStartsFromTheTemplateWhereThereIsNoFileYet(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "sshakku")

	require.NoError(t, SetKeyDir(configDir, "/home/alice/keys"))

	body, err := os.ReadFile(MainFile(configDir))
	require.NoError(t, err)
	assert.Contains(t, string(body), `key_dir = "/home/alice/keys"`)
	assert.Contains(t, string(body), "# SSHakku configuration.",
		"the commented template, so the file a user opens next still explains itself")
}

func TestSetKeyDirKeepsWhatTheUserAlreadyWrote(t *testing.T) {
	configDir := t.TempDir()
	require.NoError(t, os.WriteFile(MainFile(configDir), []byte("quiet = true\n"), 0o600))

	require.NoError(t, SetKeyDir(configDir, "/home/alice/keys"))

	body, err := os.ReadFile(MainFile(configDir))
	require.NoError(t, err)
	assert.Contains(t, string(body), "quiet = true")
	assert.Contains(t, string(body), `key_dir = "/home/alice/keys"`)
}

func TestWritableConfigLeavesNothingBehind(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "sshakku")

	require.NoError(t, WritableConfig(configDir), "a directory it may create is writable")

	entries, err := os.ReadDir(configDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "the probe file is not left for somebody to wonder about")
}

// TestADropInThatDecidesKeyDirIsNamed. Writing config.toml is how a new key
// directory is made to stick; a drop-in read afterwards would overrule it, and
// every login would go on looking in the old place with nothing to say why.
func TestADropInThatDecidesKeyDirIsNamed(t *testing.T) {
	configDir := t.TempDir()
	require.NoError(t, os.WriteFile(MainFile(configDir), []byte("key_dir = \"/one\"\n"), 0o600))
	dropIns := filepath.Join(configDir, "config.d")
	require.NoError(t, os.MkdirAll(dropIns, 0o700))
	dropIn := filepath.Join(dropIns, "50-work.toml")
	require.NoError(t, os.WriteFile(dropIn, []byte("key_dir = \"/two\"\n"), 0o600))

	decided := KeyDirDecidedElsewhere(LoadSources(configDir), os.LookupEnv, configDir)

	assert.Equal(t, dropIn, decided, "the file that would overrule what we are about to write")
}

func TestNothingIsNamedWhereTheUsersOwnFileDecides(t *testing.T) {
	configDir := t.TempDir()
	require.NoError(t, os.WriteFile(MainFile(configDir), []byte("key_dir = \"/one\"\n"), 0o600))

	assert.Empty(t, KeyDirDecidedElsewhere(LoadSources(configDir), os.LookupEnv, configDir))
}

func TestNothingIsNamedWhereNobodyHasChosenAtAll(t *testing.T) {
	assert.Empty(t, KeyDirDecidedElsewhere(LoadSources(t.TempDir()), os.LookupEnv, t.TempDir()))
}
