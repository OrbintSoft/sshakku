//go:build darwin

package main

import (
	"testing"

	"github.com/OrbintSoft/sshakku/internal/config"
)

// TestNoGraphicalPrompterOnDarwin pins today's answer on macOS: there is none,
// so every passphrase is asked for on the terminal.
//
// It is here to be deleted. A Mac in a login session always has a window server
// and `osascript … with hidden answer` would draw the prompt, so this is
// missing work rather than something the platform cannot do — recorded in
// PLAN.md Phase 17. Until then the behaviour is at least stated, and a change
// to it cannot pass unnoticed.
func TestNoGraphicalPrompterOnDarwin(t *testing.T) {
	if p := newGraphicalPrompter(config.Settings{}); p != nil {
		t.Errorf("newGraphicalPrompter = %T, want nil until macOS has a dialog of its own", p)
	}
}
