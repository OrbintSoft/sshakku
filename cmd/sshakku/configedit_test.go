package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConfigEditOpensTheUsersOwnFile verifies F36 as a user meets it: with no
// configuration at all, `sshakku config --edit` puts a file there listing what
// can be set, hands that file — and no other — to the editor, and what the
// editor saves is what SSHakku reads afterwards.
func TestConfigEditOpensTheUsersOwnFile(t *testing.T) {
	home := tempRuntimeEnv(t)
	record := useEditor(t, "")

	out, errOut, code := runConfigEdit(t)
	require.Zerof(t, code, "editing a configuration that is not there yet must work: %s / %s", out, errOut)

	t.Run("the editor was handed config.toml and nothing else", func(t *testing.T) {
		want := filepath.Join(home, ".config", "sshakku", "config.toml")
		assert.Equal(t, []string{want}, strings.Fields(readFile(t, record)),
			"the editor must be handed the user's own file and nothing else")
	})

	t.Run("the file it was handed lists what can be set", func(t *testing.T) {
		created := readFile(t, filepath.Join(home, ".config", "sshakku", "config.toml"))
		for _, key := range []string{"key_lifetime", "secret_backend", "key_patterns"} {
			assert.Containsf(t, created, key, "a file to edit must list %s among what can be set", key)
		}
	})

	t.Run("what the editor saves is what SSHakku reads", func(t *testing.T) {
		useEditor(t, "key_lifetime = \"3h\"\n")
		_, editErr, editCode := runConfigEdit(t)
		require.Zerof(t, editCode, "saving a valid file must work: %s", editErr)

		report, _, reportCode := runConfig(t)
		require.Zero(t, reportCode, "a report changes nothing and cannot fail")
		assert.Contains(t, settingLine(t, report, "key_lifetime"), "3h",
			"what the editor saved must be what SSHakku then reads")
	})
}

// TestConfigEditPassesOnTheEditorsOwnArguments covers the shape $EDITOR really
// has: people set it to a command line ("code -w", "emacs -nw"), and running
// only its first word starts some editors in a mode their owner never uses.
func TestConfigEditPassesOnTheEditorsOwnArguments(t *testing.T) {
	tempRuntimeEnv(t)
	record := useEditor(t, "")
	t.Setenv("EDITOR", editorScript(t)+" --wait --new-window")

	_, errOut, code := runConfigEdit(t)
	require.Zerof(t, code, "an $EDITOR with arguments of its own must still run: %s", errOut)

	opened := strings.Fields(readFile(t, record))
	require.Len(t, opened, 3, "the editor's own two arguments, then the path")
	assert.Equal(t, []string{"--wait", "--new-window"}, opened[:2],
		"an $EDITOR set to a command line must be run as that command line, not as its first word")
}

// TestConfigEditSaysWhatOverrulesIt verifies the half of F36 that cannot be
// seen from the file being edited: a key config.toml sets that a drop-in
// overrules is an edit with no effect, and the moment to say so is while the
// person who made it is still there.
func TestConfigEditSaysWhatOverrulesIt(t *testing.T) {
	home := tempRuntimeEnv(t)
	writeConfig(t, home, "config.d/50-work.toml", "key_lifetime = \"2h\"\n")
	useEditor(t, "key_lifetime = \"3h\"\n")

	out, errOut, code := runConfigEdit(t)
	require.Zerof(t, code, "an overruled key is not a failure: %s", errOut)

	said := out + errOut
	assert.Contains(t, said, "key_lifetime", "the edit that will have no effect must be named")
	assert.Contains(t, said, "50-work.toml", "and so must the file that overrules it")
}

// TestConfigEditReportsAValueThatWillBeIgnored verifies F36 for the file that
// parses perfectly and still does nothing: a value SSHakku will not use is
// exactly as silent as one a drop-in overrules, and it is silent in the file
// the user is looking at.
func TestConfigEditReportsAValueThatWillBeIgnored(t *testing.T) {
	tempRuntimeEnv(t)
	useEditor(t, "key_lifetime = \"eight hours\"\n")

	out, errOut, code := runConfigEdit(t)
	require.Zero(t, code, "the file is readable; one value in it is merely not usable")

	said := out + errOut
	assert.Contains(t, said, "key_lifetime", "the setting that will be ignored must be named")
	assert.Contains(t, said, "eight hours", "and the value repeated, so its author can see what they wrote")
}

// TestConfigEditReportsAFileThatNoLongerParses verifies the other half of F36:
// a syntax error discards the whole file at the next login, silently. Told at
// once, with a non-zero exit, it is a mistake the user can still fix.
func TestConfigEditReportsAFileThatNoLongerParses(t *testing.T) {
	tempRuntimeEnv(t)
	useEditor(t, "key_lifetime = \n")

	out, errOut, code := runConfigEdit(t)
	assert.Equal(t, 1, code, "a file that will be discarded at the next login must not be left to be discovered then")
	assert.Contains(t, out+errOut, "config.toml", "and the file that can no longer be read must be named")
}

// TestConfigEditWithNoEditorToRun covers the editor that is not there: naming
// it beats a silent exit, since $EDITOR is the one thing the user can correct.
func TestConfigEditWithNoEditorToRun(t *testing.T) {
	tempRuntimeEnv(t)
	t.Setenv("EDITOR", "sshakku-no-such-editor")

	_, errOut, code := runConfigEdit(t)
	assert.Equal(t, 1, code, "an editor that could not be run means nothing was edited")
	assert.Contains(t, errOut, "sshakku-no-such-editor",
		"$EDITOR is the one thing the user can correct, so it must be named")
}

// TestConfigEditFallsBackToVisual covers the second variable: a user who set
// only $VISUAL still gets an editor rather than whatever vi does on a machine
// that has none.
func TestConfigEditFallsBackToVisual(t *testing.T) {
	home := tempRuntimeEnv(t)
	useEditor(t, "max_attempts = 7\n")
	t.Setenv("EDITOR", "")
	t.Setenv("VISUAL", editorScript(t))

	_, errOut, code := runConfigEdit(t)
	require.Zerof(t, code, "a user who set only $VISUAL must still get an editor: %s", errOut)
	assert.Contains(t, readFile(t, filepath.Join(home, ".config", "sshakku", "config.toml")), "max_attempts = 7",
		"and what that editor saved must be what is on disk")
}

// TestConfigEditWithNoEditorNamedAtAll covers the last resort: a user who has
// set neither variable still gets an editor rather than an error about a
// variable they have never heard of. Asserted on the choice rather than by
// running it — vi on this machine would open on the test's own terminal and
// wait for someone to close it.
func TestConfigEditWithNoEditorNamedAtAll(t *testing.T) {
	t.Setenv("EDITOR", "")
	t.Setenv("VISUAL", "")

	assert.Equal(t, []string{fallbackEditor}, editorCommand(),
		"with neither variable set the fallback editor is used, not an error about a variable")
}

// TestConfigEditCannotMakeTheDirectoryToEditIn covers what happens when there
// is nowhere to put the file: the user is told which path could not be made,
// rather than an editor opening on nothing and their work going somewhere they
// will never find it again.
func TestConfigEditCannotMakeTheDirectoryToEditIn(t *testing.T) {
	home := tempRuntimeEnv(t)
	// A regular file where the configuration directory's parent belongs: no
	// directory can be made underneath it, whoever is running.
	require.NoError(t, os.WriteFile(filepath.Join(home, ".config"), nil, 0o600),
		"seed a file where the configuration directory's parent belongs")
	useEditor(t, "")

	_, errOut, code := runConfigEdit(t)
	assert.Equal(t, 1, code, "an editor must not open on a file that could never be saved")
	// The directory is what could not be made, and naming the file inside it
	// instead would send the user looking at the wrong thing.
	assert.Contains(t, errOut, filepath.Join(home, ".config", "sshakku"),
		"the directory that could not be made must be named")
	assert.NotContains(t, errOut, "config.toml",
		"blaming the file sends the user to look at the wrong thing")
}

// TestConfigEditCannotWriteTheFileToEdit covers the other half of the same
// promise: the directory is there but the file cannot be written, and the user
// is told so instead of being handed an editor on a file that does not exist.
func TestConfigEditCannotWriteTheFileToEdit(t *testing.T) {
	home := tempRuntimeEnv(t)
	dir := filepath.Join(home, ".config", "sshakku")
	require.NoError(t, os.MkdirAll(dir, 0o700), "create the configuration directory")
	// config.toml is a symlink to a path in a directory that is not there: it
	// is not a file that exists, and it cannot be created either — a failure
	// that does not depend on who is running the test, unlike a permission.
	require.NoError(t, os.Symlink(filepath.Join(dir, "gone", "config.toml"), filepath.Join(dir, "config.toml")),
		"point config.toml at somewhere it cannot be created")
	useEditor(t, "")

	_, errOut, code := runConfigEdit(t)
	assert.Equal(t, 1, code, "an editor must not open on a file that could not be created")
	assert.Contains(t, errOut, "config.toml", "and the file it could not create must be named")
}

// useEditor points $EDITOR at the stand-in under testdata: a real program,
// exec'd by SSHakku like any editor, which records what it was asked to open
// and saves body over it (empty body leaves the file untouched). It returns the
// path of the record.
//
// It is a script rather than a stock utility because those differ between the
// systems this suite runs on — BSD `cp` has no `-t` — and an editor is the one
// thing here that has to behave the same on all of them. Which script is this
// system's own answer, and the only thing behind a build tag:
// editorFixtureName.
func useEditor(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	record := filepath.Join(dir, "opened")
	t.Setenv("EDITOR", editorScript(t))
	t.Setenv("SSHAKKU_TEST_EDITOR_RECORD", record)
	t.Setenv("SSHAKKU_TEST_EDITOR_BODY", "")
	if body != "" {
		saved := filepath.Join(dir, "saved.toml")
		require.NoError(t, os.WriteFile(saved, []byte(body), 0o600), "write what the stand-in editor will save")
		t.Setenv("SSHAKKU_TEST_EDITOR_BODY", saved)
	}
	return record
}

// editorScript is the absolute path of the stand-in editor, which SSHakku runs
// from the user's own working directory rather than this package's.
func editorScript(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("testdata", editorFixtureName))
	require.NoError(t, err, "resolve the stand-in editor's path")
	return path
}

// runConfigEdit runs `sshakku config --edit` against the environment the test
// set up, returning stdout, stderr and the exit code.
func runConfigEdit(t *testing.T) (string, string, int) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := deps{}.run(t.Context(), &stdout, &stderr, []string{"config", "--edit"})
	return stdout.String(), stderr.String(), code
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	require.NoErrorf(t, err, "read %s", path)
	return string(body)
}
