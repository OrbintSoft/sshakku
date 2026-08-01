package keys

import "errors"

// ErrPromptCanceled is returned by a Prompter when the user dismisses the dialog
// instead of entering a passphrase. The loader treats it as "give up on this key"
// without retrying.
var ErrPromptCanceled = errors.New("passphrase prompt canceled")

// Prompter asks the user for a key's passphrase through a graphical dialog.
type Prompter interface {
	// Prompt returns the passphrase entered for keyname, ErrPromptCanceled if the
	// user dismisses the dialog, or another error if the prompt cannot run.
	Prompt(keyname string) (string, error)
	// Available reports whether the prompter's program is installed, so the loader
	// can fall back to a terminal prompt when no graphical prompter exists.
	Available() bool
}
