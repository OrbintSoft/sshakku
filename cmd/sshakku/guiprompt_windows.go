//go:build windows

package main

import (
	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys"
)

// newGraphicalPrompter returns the dialog this platform can raise a passphrase
// prompt with, and there is none here: this build draws no window, so there is
// nothing for a session with a screen to be offered. Returning nil is what
// sends the caller to the other way of asking rather than to a dialog that
// would never appear.
func newGraphicalPrompter(config.Settings, keys.Logger) keys.Prompter { return nil }
