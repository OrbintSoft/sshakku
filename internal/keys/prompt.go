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
	// Available reports whether this prompter can ask on the session it finds
	// itself in, so the loader can fall back to a terminal prompt when none can.
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

// explainedPrompter is implemented by prompters that have more than one reason
// for being unable to ask, and so cannot be reported by the usual one.
type explainedPrompter interface {
	WhyUnavailable() string
}

// PrompterUnavailable completes a sentence about p that begins with its name,
// saying why it cannot ask. Most prompters are a program that is either there or
// not, and for those there is nothing else it can be; one that knows better says
// so itself, because a reason that is not true of the machine it is read on is
// worse than none.
func PrompterUnavailable(p Prompter) string {
	if e, ok := p.(explainedPrompter); ok {
		return e.WhyUnavailable()
	}
	return "is not installed"
}
