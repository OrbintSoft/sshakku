package wallet

import "context"

// Unavailable stands for a secret store that cannot be reached at all —
// a route the user pinned that this platform does not provide, for instance.
//
// It fails every operation with the reason rather than pretending the store is
// merely empty. A miss would send the loader to prompt with no explanation and
// let a later store overwrite whatever is really in the wallet; an error says
// which route failed and why, and the loader still degrades to asking on the
// terminal rather than failing the shell.
type Unavailable struct {
	// Reason is what went wrong, phrased for the user.
	Reason error
}

// Lookup reports the reason rather than a miss.
func (b Unavailable) Lookup(context.Context, string) (string, bool, error) {
	return "", false, b.Reason
}

// Store reports the reason.
func (b Unavailable) Store(context.Context, string, string, string) error { return b.Reason }

// Delete reports the reason.
func (b Unavailable) Delete(context.Context, string) error { return b.Reason }

// List reports the reason.
func (b Unavailable) List(ctx context.Context) ([]string, error) { return nil, b.Reason }
