package prompt

import (
	"context"
	"errors"
	"fmt"
)

// FallbackPrompter asks Primary, and asks Fallback instead when Primary could
// not ask at all — a dialog that will not start, a conversation that breaks
// halfway. Being unable to ask must never cost the user the question: the answer
// is still wanted, and somewhere else can ask for it. Fallback is whatever comes
// after Primary, which may be another dialog and is a terminal in the end.
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
func (p FallbackPrompter) Prompt(ctx context.Context, keyname string) (string, error) {
	pass, err := p.Primary.Prompt(ctx, keyname)
	if err == nil || errors.Is(err, ErrCanceled) {
		return pass, err
	}
	if p.Log != nil {
		_ = p.Log.Log("ERROR", fmt.Sprintf("%s could not ask for %s (%v), asking %s instead", Name(p.Primary), keyname, err, Name(p.Fallback)))
	}
	return p.Fallback.Prompt(ctx, keyname)
}

// Name is what to call this pair in a message. Asking it is asking Primary
// first, so that is the name someone reading about it would look for; the
// halves behind it are named in their own turn, if they are ever reached.
func (p FallbackPrompter) Name() string { return Name(p.Primary) }

// Available reports whether either half can ask.
func (p FallbackPrompter) Available(ctx context.Context) bool {
	return p.Primary.Available(ctx) || p.Fallback.Available(ctx)
}

var _ Prompter = FallbackPrompter{}
