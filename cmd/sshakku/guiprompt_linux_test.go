//go:build linux

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys"
)

// TestNoGraphicalPrompterWithNoDisplayServer covers the answer a headless Linux
// session must get: no dialog, so the passphrase is asked for on the terminal
// instead. A wrong answer here is not a cosmetic one — it would send a login
// shell to draw a dialog on a display that is not there, and wait.
func TestNoGraphicalPrompterWithNoDisplayServer(t *testing.T) {
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", "")

	if p := newGraphicalPrompter(config.Settings{}, nil); p != nil {
		t.Errorf("newGraphicalPrompter = %T with no display server, want nil", p)
	}
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

	p := newGraphicalPrompter(config.Settings{}, nil)
	if p == nil {
		t.Fatal("newGraphicalPrompter = nil on a graphical session that is not KDE, so the passphrase would be asked for on the terminal instead of in a dialog")
	}
	pass, err := p.Prompt("id_test")
	if err != nil {
		t.Fatalf("Prompt = %v, want the passphrase typed into the dialog", err)
	}
	if pass != typed {
		t.Errorf("Prompt = %q, want %q", pass, typed)
	}
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

	p := newGraphicalPrompter(config.Settings{}, nil)
	fallback, ok := p.(keys.FallbackPrompter)
	if !ok {
		t.Fatalf("newGraphicalPrompter = %T with only zenity installed, want a dialog paired with the terminal", p)
	}
	if _, ok := fallback.Primary.(keys.ZenityPrompter); !ok {
		t.Errorf("the dialog asked in is %T, want zenity", fallback.Primary)
	}
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

		if p := newGraphicalPrompter(config.Settings{GUIPrompter: config.GUIPrompterNone}, nil); p != nil {
			t.Errorf("newGraphicalPrompter = %T with gui_prompter = %q, want no dialog at all", p, config.GUIPrompterNone)
		}
	})

	t.Run("a named dialog is the one asked in", func(t *testing.T) {
		install(t, "pinentry", "kdialog")

		p := newGraphicalPrompter(config.Settings{GUIPrompter: config.GUIPrompterKDialog}, nil)
		fallback, ok := p.(keys.FallbackPrompter)
		if !ok {
			t.Fatalf("newGraphicalPrompter = %T, want a dialog paired with the terminal", p)
		}
		if _, ok := fallback.Primary.(keys.KDialogPrompter); !ok {
			t.Errorf("the dialog asked in is %T, want kdialog: naming one must not get another", fallback.Primary)
		}
	})

	t.Run("a named dialog that is not installed goes to the terminal, not to another", func(t *testing.T) {
		install(t, "kdialog")

		if p := newGraphicalPrompter(config.Settings{GUIPrompter: config.GUIPrompterPinentry}, nil); p != nil {
			t.Errorf("newGraphicalPrompter = %T with only kdialog installed and pinentry named, want the terminal: a dialog the user did not choose is not a fallback", p)
		}
	})
}

// installFakeBin puts one of this package's testdata scripts into dir under the
// name a program is looked up by, so a test can compose a PATH holding exactly
// the programs the scenario says are installed.
func installFakeBin(t *testing.T, dir, name, script string) {
	t.Helper()
	body, err := os.ReadFile(script)
	if err != nil {
		t.Fatalf("reading %s: %v", script, err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), body, 0o755); err != nil {
		t.Fatalf("installing %s: %v", name, err)
	}
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
	if err := os.WriteFile(filepath.Join(dir, "kdialog"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("writing a kdialog to find: %v", err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")

	p := newGraphicalPrompter(config.Settings{}, nil)
	if p == nil {
		t.Fatal("newGraphicalPrompter = nil with a compositor advertised and kdialog installed")
	}
	fallback, ok := p.(keys.FallbackPrompter)
	if !ok {
		t.Fatalf("newGraphicalPrompter = %T, want a dialog paired with the terminal", p)
	}
	if _, ok := fallback.Primary.(keys.KDialogPrompter); !ok {
		t.Errorf("the dialog asked first is %T, want kdialog: a KDE session must not lose the dialog it already had", fallback.Primary)
	}
}
