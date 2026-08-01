//go:build linux

package main

import (
	"testing"

	"github.com/OrbintSoft/sshakku/internal/config"
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
