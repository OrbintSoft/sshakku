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

	if p := newGraphicalPrompter(config.Settings{}); p != nil {
		t.Errorf("newGraphicalPrompter = %T with no display server, want nil", p)
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

	p := newGraphicalPrompter(config.Settings{})
	if p == nil {
		t.Fatal("newGraphicalPrompter = nil with a compositor advertised and kdialog installed")
	}
	if _, ok := p.(keys.KDialogPrompter); !ok {
		t.Errorf("newGraphicalPrompter = %T, want the kdialog prompter", p)
	}
}
