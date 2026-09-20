package config

import (
	"io/fs"
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

// fakeStaged is a file that has not reached the disk: it records what was
// written to it and fails wherever it is told to, so the arms that decide
// whether a user is told their configuration was written can be reached on a
// machine where the filesystem is working perfectly well.
type fakeStaged struct {
	name     string
	written  strings.Builder
	writeErr error
	chmodErr error
	closeErr error
	closes   int
}

func (f *fakeStaged) WriteString(s string) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return f.written.WriteString(s)
}
func (f *fakeStaged) Chmod(fs.FileMode) error { return f.chmodErr }
func (f *fakeStaged) Close() error            { f.closes++; return f.closeErr }
func (f *fakeStaged) Name() string            { return f.name }

// stagedBy puts the given file (or failure) in place of the real one for the
// length of one test.
func stagedBy(t *testing.T, f *fakeStaged, err error) {
	t.Helper()
	original := createTemp
	createTemp = func(string, string) (stagedFile, error) { return f, err }
	t.Cleanup(func() { createTemp = original })
}

func TestKeyDirIsTheWholeFileWhereThereWasNothingAtAll(t *testing.T) {
	assert.Equal(t, "key_dir = \"/home/alice/keys\"\n", WithKeyDir("", "/home/alice/keys"),
		"an empty file becomes one that sets exactly this and nothing else")
}

func TestKeyDirIsAppendedToAFileThatDoesNotEndInANewline(t *testing.T) {
	body := WithKeyDir("max_attempts = 3", "/home/alice/keys")

	assert.Contains(t, body, "max_attempts = 3\n", "what was there keeps its own line")
	assert.Contains(t, body, "key_dir = \"/home/alice/keys\"\n")
	assert.NotContains(t, body, "3key_dir", "rather than being run onto the end of it")
}

// TestAConfigurationThatCannotBeReadIsNotOverwritten. Starting again from the
// template would throw away everything the user wrote, on a machine where the
// only thing established is that their file could not be read.
func TestAConfigurationThatCannotBeReadIsNotOverwritten(t *testing.T) {
	configDir := t.TempDir()
	require.NoError(t, os.MkdirAll(MainFile(configDir), 0o700), "something there that is not a file")

	err := SetKeyDir(configDir, "/home/alice/keys")

	require.Error(t, err, "a file that could not be read is not a file that is not there")
	assert.Contains(t, err.Error(), MainFile(configDir), "and the path is named")
}

// TestAConfigurationDirectoryThatCannotBeMadeIsRefusedBeforeAndDuring, by both
// the question asked ahead of a move and the write itself.
func TestAConfigurationDirectoryThatCannotBeMadeIsRefusedBeforeAndDuring(t *testing.T) {
	root := t.TempDir()
	inTheWay := filepath.Join(root, "not-a-directory")
	require.NoError(t, os.WriteFile(inTheWay, nil, 0o600))
	configDir := filepath.Join(inTheWay, "sshakku")

	assert.Error(t, WritableConfig(configDir), "the question asked before anything moves")
	assert.Error(t, SetKeyDir(configDir, "/home/alice/keys"), "and the write itself")
}

// TestAFileThatCouldNotBeMadeToWriteThroughIsReported covers the failure a
// filesystem produces rather than a caller: no room, or a directory that
// stopped being writable while this was going on.
func TestAFileThatCouldNotBeMadeToWriteThroughIsReported(t *testing.T) {
	configDir := t.TempDir()
	stagedBy(t, nil, assert.AnError)

	assert.Error(t, SetKeyDir(configDir, "/home/alice/keys"), "nothing was staged, so nothing was written")
	assert.Error(t, WritableConfig(configDir), "and the same answer ahead of time")
}

// TestAConfigurationThatCouldNotBeWrittenSaysSoAndLeavesNothingOpen. Each of
// the three ways a staged file fails is reported, and the file is closed
// whichever way it went: this runs at every move of somebody's keys.
func TestAConfigurationThatCouldNotBeWrittenSaysSoAndLeavesNothingOpen(t *testing.T) {
	tests := []struct {
		name string
		file *fakeStaged
	}{
		{"nothing could be written into it", &fakeStaged{writeErr: assert.AnError}},
		{"it could not be made private", &fakeStaged{chmodErr: assert.AnError}},
		{"it could not be closed", &fakeStaged{closeErr: assert.AnError}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			configDir := t.TempDir()
			tc.file.name = filepath.Join(configDir, "config.toml.staged")
			stagedBy(t, tc.file, nil)

			err := SetKeyDir(configDir, "/home/alice/keys")

			require.Error(t, err)
			assert.Contains(t, err.Error(), MainFile(configDir), "the file it was writing is named")
			assert.Positive(t, tc.file.closes, "and nothing is left open behind it")
			assert.NoFileExists(t, MainFile(configDir), "the configuration is not half-written")
		})
	}
}

// TestAConfigurationThatCouldNotBeMovedIntoPlaceIsReported. The staged file is
// written and then renamed over the real one, and a rename that fails leaves
// the user's own file untouched — which is right, and has to be said.
func TestAConfigurationThatCouldNotBeMovedIntoPlaceIsReported(t *testing.T) {
	configDir := t.TempDir()
	require.NoError(t, os.WriteFile(MainFile(configDir), []byte("key_lifetime = \"2h\"\n"), 0o600))
	original := renameFile
	renameFile = func(string, string) error { return assert.AnError }
	t.Cleanup(func() { renameFile = original })

	err := SetKeyDir(configDir, "/home/alice/keys")

	require.Error(t, err)
	kept, readErr := os.ReadFile(MainFile(configDir))
	require.NoError(t, readErr)
	assert.Equal(t, "key_lifetime = \"2h\"\n", string(kept), "what the user wrote is still what is there")
}

// TestAProbeThatCouldNotBeClosedOrRemovedIsAFailedQuestion. WritableConfig
// answers by writing, so a probe it cannot finish with is an answer it did not
// get — and reporting "yes, writable" there would let a key move go ahead on
// the strength of a question nobody answered.
func TestAProbeThatCouldNotBeClosedOrRemovedIsAFailedQuestion(t *testing.T) {
	t.Run("a probe that could not be closed", func(t *testing.T) {
		configDir := t.TempDir()
		probe := &fakeStaged{name: filepath.Join(configDir, "probe"), closeErr: assert.AnError}
		stagedBy(t, probe, nil)
		removed := false
		original := removeFile
		removeFile = func(string) error { removed = true; return nil }
		t.Cleanup(func() { removeFile = original })

		assert.Error(t, WritableConfig(configDir))
		assert.True(t, removed, "and the probe is taken away rather than left for somebody to wonder about")
	})

	t.Run("a probe that could not be taken away", func(t *testing.T) {
		configDir := t.TempDir()
		stagedBy(t, &fakeStaged{name: filepath.Join(configDir, "probe")}, nil)
		original := removeFile
		removeFile = func(string) error { return assert.AnError }
		t.Cleanup(func() { removeFile = original })

		assert.Error(t, WritableConfig(configDir),
			"a directory this cannot tidy up after itself in is not one to start a migration in")
	})
}

// TestAConfigurationDirectoryThatGoesAwayMidWriteIsReported. The directory is
// asked for before the file is read and made after it, so a directory that
// stops being there in between — somebody else's tidying, a home unmounted — is
// a write that cannot happen and must not be reported as one that did.
func TestAConfigurationDirectoryThatGoesAwayMidWriteIsReported(t *testing.T) {
	configDir := t.TempDir()
	original := makeDir
	makeDir = func(string, fs.FileMode) error { return assert.AnError }
	t.Cleanup(func() { makeDir = original })

	err := SetKeyDir(configDir, "/home/alice/keys")

	require.Error(t, err)
	assert.Contains(t, err.Error(), configDir, "the directory it could not make is named")
	assert.NoFileExists(t, MainFile(configDir), "and nothing was written into it")
}
