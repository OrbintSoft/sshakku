//go:build darwin

package main

import (
	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys"
)

// newGraphicalPrompter returns no dialog on macOS, so every passphrase this
// platform has to ask for is asked on the terminal.
//
// This is a gap, not an absence. A Mac in a login session always has a window
// server, and `osascript -e 'display dialog … with hidden answer'` would draw
// the prompt with nothing extra installed; what does not exist is an
// implementation of it here. Returning nil keeps the behaviour SSHakku has
// always had on this platform — the Linux detection could only ever answer
// "no", since it looks for WAYLAND_DISPLAY, an X server and kdialog, none of
// which a Mac has — while stating it, so it can be found and fixed rather than
// being an accident of a missing binary.
func newGraphicalPrompter(config.Settings) keys.Prompter { return nil }
