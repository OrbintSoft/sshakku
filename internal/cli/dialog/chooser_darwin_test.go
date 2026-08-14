//go:build darwin

package dialog

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These verify F29 on macOS: asked in a dialog where the session can show one,
// asked on the terminal where it cannot.
//
// Both conditions are supplied as what they really are — what the system says
// about the session, and what is installed to draw with — rather than stubbed.
// A fake `launchctl` on PATH answers the way the real one does in each kind of
// session, and the judgement of that answer is the code under test.

// fakeTool writes an executable of the given name into dir, printing out.
func fakeTool(t *testing.T, dir, name, out string) {
	t.Helper()
	script := "#!/bin/sh\nprintf '%s\\n' " + "'" + out + "'\n"
	require.NoErrorf(t, os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755), "writing a fake %s", name)
}

// TestGraphicalPrompterInAGraphicalSession is the case macOS has never had: a
// login at the machine's own screen, where `launchctl managername` answers
// Aqua and a dialog can be drawn.
func TestGraphicalPrompterInAGraphicalSession(t *testing.T) {
	dir := t.TempDir()
	fakeTool(t, dir, "launchctl", "Aqua")
	fakeTool(t, dir, "osascript", "")
	t.Setenv("PATH", dir)

	assert.NotNil(t, Graphical(t.Context(), config.Settings{}, nil),
		"a login at the machine's own screen can be shown a dialog")
}

// TestNoGraphicalPrompterOutsideAGraphicalSession covers the answer that keeps
// a login shell from hanging: an SSH login and a single-user boot are both
// sessions with no window server, and `launchctl managername` says so. Being
// on a Mac is not the condition — this is what F29 forbids getting wrong.
func TestNoGraphicalPrompterOutsideAGraphicalSession(t *testing.T) {
	for _, manager := range []string{"Background", "StandardIO", "System"} {
		t.Run(manager, func(t *testing.T) {
			dir := t.TempDir()
			fakeTool(t, dir, "launchctl", manager)
			fakeTool(t, dir, "osascript", "")
			t.Setenv("PATH", dir)

			assert.Nilf(t, Graphical(t.Context(), config.Settings{}, nil),
				"a %s session has no window server, so there is nowhere to draw a dialog", manager)
		})
	}
}

// TestNoGraphicalPrompterWithNothingToDrawWith completes the pair the Linux
// side already has: a session that could show a dialog, and nothing installed
// to draw one.
func TestNoGraphicalPrompterWithNothingToDrawWith(t *testing.T) {
	dir := t.TempDir()
	fakeTool(t, dir, "launchctl", "Aqua")
	t.Setenv("PATH", dir)

	assert.Nil(t, Graphical(t.Context(), config.Settings{}, nil),
		"a session that could show a dialog still needs something installed to draw one")
}

// TestNoGraphicalPrompterWhenTheUserRefusedOne verifies F37 on macOS: refusing
// a dialog is the user's to write, and it holds where a dialog could perfectly
// well have been shown — a session with a screen and osascript installed.
func TestNoGraphicalPrompterWhenTheUserRefusedOne(t *testing.T) {
	dir := t.TempDir()
	fakeTool(t, dir, "launchctl", "Aqua")
	fakeTool(t, dir, "osascript", "")
	t.Setenv("PATH", dir)

	settings := config.Settings{GUIPrompter: config.GUIPrompterNone}
	assert.Nil(t, Graphical(t.Context(), settings, nil),
		"refusing a dialog is the user's to write, and it holds where one could have been shown")
}
