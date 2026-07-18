//go:build unix

package main

import "github.com/OrbintSoft/sshakku/internal/keys"

// ttyPrompter reads one line from the controlling terminal (/dev/tty),
// optionally with echo disabled, as the askpass broker's fallback. Reaching
// /dev/tty rather than stdin works even though ssh runs the askpass helper
// detached from stdin.
type ttyPrompter struct{}

func (ttyPrompter) Prompt(prompt string, secret bool) (string, error) {
	return keys.ReadTTYLine(prompt, secret)
}
