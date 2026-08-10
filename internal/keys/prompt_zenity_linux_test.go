//go:build linux

package keys

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestZenityPrompt(t *testing.T) {
	t.Run("returns the entered passphrase, newline trimmed", func(t *testing.T) {
		r := newFakeRunner().on("zenity", stdout("typed-pass\n", 0))
		got, err := ZenityPrompter{Runner: r}.Prompt("id_rsa")
		require.NoError(t, err, "a dialog the user answered must hand the answer back")
		assert.Equal(t, "typed-pass", got,
			"and it must be what they typed: the newline the dialog prints is not part of the passphrase")
		require.NotEmpty(t, r.calls, "the dialog must actually be run")
		assert.Equal(t, []string{"--password", "--title", "Enter passphrase for id_rsa"}, r.calls[0].Args,
			"asked as a password, so the characters do not appear on screen, and naming the key it is for")
	})

	t.Run("a dismissed dialog is ErrPromptCanceled", func(t *testing.T) {
		r := newFakeRunner().on("zenity", stdout("", 1))
		_, err := ZenityPrompter{Runner: r}.Prompt("id_rsa")
		assert.ErrorIs(t, err, ErrPromptCanceled,
			"closing a dialog is an answer, and must be passed on as one rather than as a failure")
	})

	t.Run("a failure to start zenity is an error", func(t *testing.T) {
		wantErr := errors.New("boom")
		r := newFakeRunner().on("zenity", fails(wantErr))
		_, err := ZenityPrompter{Runner: r}.Prompt("id_rsa")
		assert.ErrorIs(t, err, wantErr,
			"a dialog that could not be started must be reported as that, not as one the user dismissed")
	})
}

func TestZenityAvailable(t *testing.T) {
	found := ZenityPrompter{lookPath: func(string) (string, error) { return "/usr/bin/zenity", nil }}
	assert.True(t, found.Available(), "a dialog that is installed can be asked in")
	missing := ZenityPrompter{lookPath: func(string) (string, error) { return "", errors.New("not found") }}
	assert.False(t, missing.Available(), "and one that is not installed cannot")
}

// TestZenityAvailableDefaultLookPath covers Available's nil-lookPath branch,
// which falls back to the real os/exec PATH lookup. The result depends on
// whether zenity happens to be installed; only the branch matters here.
func TestZenityAvailableDefaultLookPath(t *testing.T) {
	_ = ZenityPrompter{}.Available()
}
