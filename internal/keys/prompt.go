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

// namedPrompter is implemented by prompters that can say what they are, so a
// message about one names the program the user would go and look for.
type namedPrompter interface {
	Name() string
}

// PrompterName is what to call p in a message. A prompter that does not say is
// described by what it is rather than left unnamed.
func PrompterName(p Prompter) string {
	if n, ok := p.(namedPrompter); ok {
		return n.Name()
	}
	return "the passphrase dialog"
}
