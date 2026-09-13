//go:build windows

package dialog

import (
	"github.com/OrbintSoft/sshakku/internal/keys/prompt"
)

// TTY reads one line from this session's console, optionally with echo
// disabled, as the askpass broker's fallback. The console is reached by name
// (CONIN$ and CONOUT$) rather than through standard input, which is what lets
// the question be asked in a process ssh started with no stdin at all. A
// session with no console is reported as such, and the broker says so instead
// of asking.
type TTY struct{}

func (TTY) Prompt(question string, secret bool) (string, error) {
	// Reads the real console (CONIN$/CONOUT$); this is the production
	// prompt.TTY implementation that unit tests replace with a fake, so its body
	// cannot run in a unit test.
	//coverage:ignore
	return prompt.ReadTTYLine(question, secret)
}
