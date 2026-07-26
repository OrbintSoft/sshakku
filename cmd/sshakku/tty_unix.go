//go:build unix

package main

import "github.com/OrbintSoft/sshakku/internal/keys"

// ttyPrompter reads one line from the controlling terminal (/dev/tty),
// optionally with echo disabled, as the askpass broker's fallback. Reaching
// /dev/tty rather than stdin works even though ssh runs the askpass helper
// detached from stdin.
type ttyPrompter struct{}

func (ttyPrompter) Prompt(prompt string, secret bool) (string, error) {
	// Reads the real controlling terminal (/dev/tty); this is the production TTY
	// implementation that unit tests replace with a fake, so its body cannot run
	// in a unit test.
	//coverage:ignore
	return keys.ReadTTYLine(prompt, secret)
}
