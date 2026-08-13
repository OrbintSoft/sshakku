//go:build darwin

package keys

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOsascriptPrompt(t *testing.T) {
	t.Run("returns the entered passphrase, newline trimmed", func(t *testing.T) {
		r := newFakeRunner().on(osascriptBin, stdout("typed-pass\n", 0))
		got, err := OsascriptPrompter{Runner: r}.Prompt(t.Context(), "id_rsa")
		require.NoError(t, err, "a dialog the user answered must hand the answer back")
		assert.Equal(t, "typed-pass", got,
			"and it must be what they typed: the newline the dialog prints is not part of the passphrase")

		require.NotEmpty(t, r.calls, "osascript must actually be run")
		args := r.calls[0].Args
		require.Lenf(t, args, 2, "with a script to run and the key name to put in it: %v", args)
		// The key name is an argument, not part of the script: a name that
		// closed the string it had been pasted into would otherwise continue
		// as AppleScript of its own.
		assert.Equal(t, "id_rsa", args[1],
			"the key name travels as an argument: pasted into the script, a name that closed the string it sat in "+
				"would carry on as AppleScript of its own")
		assert.True(t, strings.HasSuffix(args[0], ".applescript"), "and the first argument is the script to run")
	})

	t.Run("the script really is there while the dialog runs", func(t *testing.T) {
		var body []byte
		r := newFakeRunner().on(osascriptBin, func(c Cmd) (Result, error) {
			body, _ = os.ReadFile(c.Args[0])
			return Result{Stdout: []byte("x")}, nil
		})
		_, err := OsascriptPrompter{Runner: r}.Prompt(t.Context(), "id_rsa")
		require.NoError(t, err, "putting the dialog on the screen must succeed")
		assert.Equal(t, passphraseDialog, string(body),
			"and what osascript reads while it runs must be the dialog SSHakku ships, present on disk at that moment")
	})

	t.Run("the script does not outlive the prompt", func(t *testing.T) {
		var path string
		r := newFakeRunner().on(osascriptBin, func(c Cmd) (Result, error) {
			path = c.Args[0]
			return Result{Stdout: []byte("x")}, nil
		})
		_, err := OsascriptPrompter{Runner: r}.Prompt(t.Context(), "id_rsa")
		require.NoError(t, err, "putting the dialog on the screen must succeed")
		_, err = os.Stat(path)
		assert.ErrorIsf(t, err, os.ErrNotExist, "and the script must not outlive the prompt: %s", path)
	})

	t.Run("a dismissed dialog is ErrPromptCanceled", func(t *testing.T) {
		r := newFakeRunner().on(osascriptBin, stdout("", 1))
		_, err := OsascriptPrompter{Runner: r}.Prompt(t.Context(), "id_rsa")
		assert.ErrorIs(t, err, ErrPromptCanceled,
			"closing a dialog is an answer, and must be passed on as one rather than as a failure")
	})

	t.Run("a failure to start osascript is an error", func(t *testing.T) {
		wantErr := errors.New("boom")
		r := newFakeRunner().on(osascriptBin, fails(wantErr))
		_, err := OsascriptPrompter{Runner: r}.Prompt(t.Context(), "id_rsa")
		assert.ErrorIs(t, err, wantErr,
			"a dialog that could not be started must be reported as that, not as one the user dismissed")
	})

	t.Run("a script file that cannot be created is an error, not a silent no-prompt", func(t *testing.T) {
		wantErr := errors.New("read-only file system")
		defer stubCreateDialogScript(t, func() (*os.File, error) { return nil, wantErr })()

		r := newFakeRunner().on(osascriptBin, stdout("typed-pass\n", 0))
		_, err := OsascriptPrompter{Runner: r}.Prompt(t.Context(), "id_rsa")
		assert.ErrorIs(t, err, wantErr,
			"with no script to put on the screen the question cannot be asked, and that must be said rather than "+
				"answered as though nobody was there")
		assert.Emptyf(t, r.calls, "and osascript must not be run with nothing to run: %+v", r.calls)
	})

	t.Run("a script that cannot be written is an error, and leaves nothing behind", func(t *testing.T) {
		// A file already closed is one every write fails on, which is what a
		// real temporary directory will not do on request.
		var path string
		defer stubCreateDialogScript(t, func() (*os.File, error) {
			f, err := os.CreateTemp(t.TempDir(), "closed-*.applescript")
			if err != nil {
				return nil, err
			}
			path = f.Name()
			return f, f.Close()
		})()

		r := newFakeRunner().on(osascriptBin, stdout("typed-pass\n", 0))
		_, err := OsascriptPrompter{Runner: r}.Prompt(t.Context(), "id_rsa")
		assert.Error(t, err, "a script that could not be written cannot be run, and that must be said")
		assert.Emptyf(t, r.calls, "osascript must not be run with a script that was never written: %+v", r.calls)
		_, statErr := os.Stat(path)
		assert.ErrorIsf(t, statErr, os.ErrNotExist, "and nothing may be left behind: %s", path)
	})
}

// stubCreateDialogScript swaps the file-creation seam for the duration of a
// test and returns the restore.
func stubCreateDialogScript(t *testing.T, f func() (*os.File, error)) func() {
	t.Helper()
	restore := createDialogScript
	createDialogScript = f
	return func() { createDialogScript = restore }
}

func TestOsascriptAvailable(t *testing.T) {
	found := OsascriptPrompter{lookPath: func(string) (string, error) { return "/usr/bin/osascript", nil }}
	assert.True(t, found.Available(t.Context()), "a dialog that is installed can be asked in")
	missing := OsascriptPrompter{lookPath: func(string) (string, error) { return "", errors.New("not found") }}
	assert.False(t, missing.Available(t.Context()), "and one that is not installed cannot")
}

// TestOsascriptAvailableDefaultLookPath covers Available's nil-lookPath branch,
// which falls back to the real os/exec PATH lookup.
func TestOsascriptAvailableDefaultLookPath(t *testing.T) {
	_ = OsascriptPrompter{}.Available(t.Context())
}

// TestEmbeddedDialogTakesItsArgument pins the contract the Go side depends on:
// the script reads the key name from argv rather than having it built in, and
// hides what is typed. Compiling it is the macOS lint target's job; what is
// checked here is that these two properties are not quietly dropped.
func TestEmbeddedDialogTakesItsArgument(t *testing.T) {
	assert.Contains(t, passphraseDialog, "on run argv",
		"the key name reaches the dialog as an argument rather than being built into it")
	assert.Contains(t, passphraseDialog, "with hidden answer",
		"and what is typed is hidden, or the passphrase is on screen for anyone behind the user")
}
