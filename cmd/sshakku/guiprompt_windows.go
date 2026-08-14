//go:build windows

package main

import (
	"context"

	"github.com/OrbintSoft/sshakku/internal/config"
	"github.com/OrbintSoft/sshakku/internal/keys"
	"github.com/OrbintSoft/sshakku/internal/keys/prompt"
)

// newGraphicalPrompter returns the dialog this platform can raise a passphrase
// prompt with, and there is none here: this build draws no window, so there is
// nothing for a session with a screen to be offered. Returning nil is what
// sends the caller to the other way of asking rather than to a dialog that
// would never appear.
func newGraphicalPrompter(context.Context, config.Settings, keys.Logger) prompt.Prompter { return nil }
