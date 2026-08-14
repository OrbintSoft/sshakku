//go:build windows

package main

import (
	"github.com/OrbintSoft/sshakku/internal/keys/prompt"
)

// ttyPrompter is the askpass broker's fallback: one line read from the
// terminal the user is actually at. It defers to prompt.ReadTTYLine, which on
// this platform reports that prompting on the console is not implemented — the
// broker then reports that reason instead of asking.
type ttyPrompter struct{}

func (ttyPrompter) Prompt(question string, secret bool) (string, error) {
	return prompt.ReadTTYLine(question, secret)
}
