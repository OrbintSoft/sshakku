package keys

import (
	"errors"
	"fmt"
)

// FallbackPrompter asks Primary, and asks Fallback instead when Primary could
// not ask at all — a dialog that will not start, a conversation that breaks
// halfway. Being unable to ask must never cost the user the question: the answer
// is still wanted, and there is a terminal to ask it on.
//
// A dismissed dialog is not that case. Cancelling is an answer, and it is
// propagated as one: asking again somewhere else would be overruling a decision
// the user just made.
type FallbackPrompter struct {
	Primary  Prompter
	Fallback Prompter
	// Log records which prompter failed and why, so a dialog that never appears
	// is something the user can find out about rather than guess at. Nil records
	// nothing.
	Log Logger
}

// Prompt asks for keyname's passphrase.
func (p FallbackPrompter) Prompt(keyname string) (string, error) {
	pass, err := p.Primary.Prompt(keyname)
	if err == nil || errors.Is(err, ErrPromptCanceled) {
		return pass, err
	}
	if p.Log != nil {
		_ = p.Log.Log("ERROR", fmt.Sprintf("%s could not ask for %s (%v), asking on the terminal instead", PrompterName(p.Primary), keyname, err))
	}
	return p.Fallback.Prompt(keyname)
}

// Available reports whether either half can ask.
func (p FallbackPrompter) Available() bool {
	return p.Primary.Available() || p.Fallback.Available()
}

var _ Prompter = FallbackPrompter{}
