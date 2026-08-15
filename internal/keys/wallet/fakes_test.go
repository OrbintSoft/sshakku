package wallet

import (
	"context"
)

// fakePrompter is a prompt.Prompter whose answer is scripted: the wallets that
// can only be opened with a password of their own ask through one, and a test
// about what a wallet does with that password must not depend on a person
// being there to type it.
type fakePrompter struct {
	avail bool
	pass  string
	err   error
	calls []string
}

func (p *fakePrompter) Available(context.Context) bool { return p.avail }

func (p *fakePrompter) Prompt(_ context.Context, keyname string) (string, error) {
	p.calls = append(p.calls, keyname)
	return p.pass, p.err
}
