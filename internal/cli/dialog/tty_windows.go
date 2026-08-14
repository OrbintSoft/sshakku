//go:build windows

package dialog

import (
	"github.com/OrbintSoft/sshakku/internal/keys/prompt"
)

// TTY is the askpass broker's fallback: one line read from the
// terminal the user is actually at. It defers to prompt.ReadTTYLine, which on
// this platform reports that prompting on the console is not implemented — the
// broker then reports that reason instead of asking.
type TTY struct{}

func (TTY) Prompt(question string, secret bool) (string, error) {
	return prompt.ReadTTYLine(question, secret)
}
