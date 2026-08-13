//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNoGraphicalPrompterWithNoDisplayServer covers the answer a headless Linux
// session must get: no dialog, so the passphrase is asked for on the terminal
// instead. A wrong answer here is not a cosmetic one — it would send a login
// shell to draw a dialog on a display that is not there, and wait.
func TestNoGraphicalPrompterWithNoDisplayServer(t *testing.T) {
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", "")

	assert.Nil(t, newGraphicalPrompter(t.Context(), config.Settings{}, nil),
		"with no display server a dialog would be drawn nowhere, and the login shell would wait for it")
}

// fakePinentry is a program that speaks pinentry's line protocol for real, so a
// test can put "pinentry is installed" on PATH without needing the machine
// running the suite to have one.
const fakePinentry = "../../test/fakes/pinentry.sh"

// TestGraphicalPromptOnADesktopWithoutKDE verifies F29: sitting at the machine,
// the prompt is a dialog. Most graphical sessions are not KDE ones, and a user
// who has never installed KDE's tooling still has a screen to put a dialog on —
// so being asked on the terminal here is the promise not being kept, not a
// milder form of keeping it. The terminal a login shell is asked on may be
// behind the window the user is actually looking at, or scrolled past.
//
// The session and the installed dialog are supplied as what they are — things
// read from outside the process — rather than stubbed: a compositor advertises
// itself the way a compositor does, and pinentry is on PATH the way installing
// it would put it there, with nothing else beside it. What decides which
// prompter comes back, and what it does when asked, is the real code.
func TestGraphicalPromptOnADesktopWithoutKDE(t *testing.T) {
	const typed = "the-one-typed-into-the-dialog"

	dir := t.TempDir()
	installFakeBin(t, dir, "pinentry", fakePinentry)
	t.Setenv("PATH", dir)
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	t.Setenv("SSHAKKU_TEST_PINENTRY_PIN", typed)

	p := newGraphicalPrompter(t.Context(), config.Settings{}, nil)
	require.NotNil(t, p,
		"a graphical session that is not KDE still has a screen, so the passphrase is asked for in a dialog")

	pass, err := p.Prompt(t.Context(), "id_test")
	require.NoError(t, err, "the dialog must answer")
	assert.Equal(t, typed, pass, "with what was typed into it")
}

// TestGraphicalPromptWithOnlyZenity verifies F29 on the desktops that have
// neither GnuPG's dialog nor KDE's: a GTK session where zenity is what is
// installed still has a screen, and being asked on the terminal there is the
// promise not being kept.
func TestGraphicalPromptWithOnlyZenity(t *testing.T) {
	dir := t.TempDir()
	installFakeBin(t, dir, "zenity", fakePinentry)
	t.Setenv("PATH", dir)
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")

	p := newGraphicalPrompter(t.Context(), config.Settings{}, nil)
	fallback, ok := p.(keys.FallbackPrompter)
	require.Truef(t, ok, "a dialog must be paired with the terminal to fall back to, got %T", p)
	assert.IsType(t, keys.ZenityPrompter{}, fallback.Primary,
		"zenity is what is installed, so zenity is what asks")
}

// TestGraphicalPromptWhereTheOnlyPinentryIsAConsoleOne verifies F29 on a session
// that has a screen and a dialog to put on it, but whose pinentry is one of
// GnuPG's console builds. Which build a machine has is the distribution's
// choice, not the user's, and a user who has one that draws on a terminal has
// not thereby asked to be prompted on a terminal: the screen is still there and
// zenity is still installed, so a dialog is still what the promise says they
// meet.
//
// The console pinentry is staged as what it is — a program that says which
// builds it can draw with when asked, the way pinentry answers that question —
// rather than by telling the code which prompter to skip.
func TestGraphicalPromptWhereTheOnlyPinentryIsAConsoleOne(t *testing.T) {
	dir := t.TempDir()
	installFakeBin(t, dir, "pinentry", fakePinentry)
	installFakeBin(t, dir, "zenity", fakePinentry)
	t.Setenv("PATH", dir)
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	t.Setenv("SSHAKKU_TEST_PINENTRY_FLAVOR", "curses")

	p := newGraphicalPrompter(t.Context(), config.Settings{}, nil)
	fallback, ok := p.(keys.FallbackPrompter)
	require.Truef(t, ok, "a dialog must be paired with the terminal to fall back to, got %T", p)
	assert.IsType(t, keys.ZenityPrompter{}, fallback.Primary,
		"a pinentry that draws on a terminal is not a dialog, and taking the prompt from one that is leaves the screen empty")
}

// TestGraphicalPrompterHonoursTheConfiguration verifies F37: which dialog asks
// is the user's to choose, and choosing one is never a way to lose the prompt.
//
// Each case stages a session that could show a dialog and the programs that are
// installed in it, then states what the configuration says. What is asserted is
// what the user would meet: a dialog, a named dialog, or the terminal.
func TestGraphicalPrompterHonoursTheConfiguration(t *testing.T) {
	install := func(t *testing.T, names ...string) {
		t.Helper()
		dir := t.TempDir()
		for _, name := range names {
			installFakeBin(t, dir, name, fakePinentry)
		}
		t.Setenv("PATH", dir)
		t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	}

	t.Run("none asks on the terminal even with a screen and a dialog installed", func(t *testing.T) {
		install(t, "pinentry", "kdialog")

		assert.Nilf(t, newGraphicalPrompter(t.Context(), config.Settings{GUIPrompter: config.GUIPrompterNone}, nil),
			"gui_prompter = %q means no dialog, screen or no screen", config.GUIPrompterNone)
	})

	t.Run("a named dialog is the one asked in", func(t *testing.T) {
		install(t, "pinentry", "kdialog")

		p := newGraphicalPrompter(t.Context(), config.Settings{GUIPrompter: config.GUIPrompterKDialog}, nil)
		fallback, ok := p.(keys.FallbackPrompter)
		require.Truef(t, ok, "a dialog must be paired with the terminal to fall back to, got %T", p)
		assert.IsType(t, keys.KDialogPrompter{}, fallback.Primary, "naming one dialog must not get another")
	})

	t.Run("a named dialog that is not installed goes to the terminal, not to another", func(t *testing.T) {
		install(t, "kdialog")

		assert.Nil(t, newGraphicalPrompter(t.Context(), config.Settings{GUIPrompter: config.GUIPrompterPinentry}, nil),
			"a dialog the user did not choose is not a fallback for the one they did")
	})
}

// TestGraphicalSessionWithNoDialogInstalledAsksOnTheTerminal covers the other
// way F29 can be kept: a screen is not enough on its own, and a session that has
// none of the three dialogs installed has to be asked on the terminal rather
// than left with a question that appears nowhere.
func TestGraphicalSessionWithNoDialogInstalledAsksOnTheTerminal(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")

	assert.Nil(t, newGraphicalPrompter(t.Context(), config.Settings{}, nil),
		"a screen is not enough on its own: with no dialog installed the question must go to the terminal")
}

// TestANamedPinentryThatCannotDrawIsNotReportedMissing verifies the half of F37
// that is a sentence: naming a dialog that cannot ask here sends the user to the
// terminal and tells them which one failed. What is written down has to be true
// of the machine it was written on — telling someone that pinentry is not
// installed sends them to install what they already have, and leaves the reason
// they were not asked in a window exactly as hidden as it was before.
func TestANamedPinentryThatCannotDrawIsNotReportedMissing(t *testing.T) {
	dir := t.TempDir()
	installFakeBin(t, dir, "pinentry", fakePinentry)
	t.Setenv("PATH", dir)
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	t.Setenv("SSHAKKU_TEST_PINENTRY_FLAVOR", "curses")

	log := &recordingLogger{}
	assert.Nil(t, newGraphicalPrompter(t.Context(), config.Settings{GUIPrompter: config.GUIPrompterPinentry}, log),
		"a named pinentry that cannot draw here sends the question to the terminal")

	said := strings.Join(log.lines, "\n")
	assert.Contains(t, said, config.GUIPrompterPinentry, "the log must name the dialog that could not ask")
	assert.Contains(t, said, "screen",
		"and allow for the case it is in: one that is installed and simply cannot draw where the user is looking")
}

// installFakeBin puts one of this package's testdata scripts into dir under the
// name a program is looked up by, so a test can compose a PATH holding exactly
// the programs the scenario says are installed.
func installFakeBin(t *testing.T, dir, name, script string) {
	t.Helper()
	body, err := os.ReadFile(script)
	require.NoErrorf(t, err, "reading %s", script)
	require.NoErrorf(t, os.WriteFile(filepath.Join(dir, name), body, 0o755), "installing %s", name)
}

// TestGraphicalPrompterWithASessionAndKDialog covers the opposite answer, which
// no machine running this suite had ever produced: a hosted runner has no
// display server, so the branch that hands the dialog back had never been taken
// anywhere the result was measured.
//
// Both conditions are supplied as what they really are — things read from
// outside the process — rather than stubbed: a compositor is advertised the way
// a compositor advertises itself, and kdialog is put on PATH the way installing
// it would put it there. What decides remains the real GUIAvailable.
func TestGraphicalPrompterWithASessionAndKDialog(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kdialog"), []byte("#!/bin/sh\nexit 0\n"), 0o755),
		"writing a kdialog to find")
	t.Setenv("PATH", dir)
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")

	p := newGraphicalPrompter(t.Context(), config.Settings{}, nil)
	require.NotNil(t, p, "a compositor is advertised and kdialog is installed, so there is a dialog to ask in")
	fallback, ok := p.(keys.FallbackPrompter)
	require.Truef(t, ok, "a dialog must be paired with the terminal to fall back to, got %T", p)
	assert.IsType(t, keys.KDialogPrompter{}, fallback.Primary,
		"a KDE session must not lose the dialog it already had")
}
