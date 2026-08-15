package crossuser

import "context"

// Source reads another user's per-login socket token. A kernel
// keyring is only visible to the uid that owns it, unlike files, which root can
// read regardless of owner, so an implementation must itself assume the target
// uid's identity to succeed — that privilege transition is ReadToken's
// responsibility, not its caller's.
type Source interface {
	// ReadToken returns the target uid/gid's socket token, or "" when none
	// exists yet (a valid, tokenless state, not an error).
	ReadToken(ctx context.Context, uid, gid int) (string, error)
}

// ReadSocketTokenCmd is not a user-facing command: `doctor` runs the
// binary with this as its argument, as a child holding another user's credentials,
// to read that user's per-login socket token from their own kernel keyring (a
// keyring is only visible to the uid that owns it, unlike files, which root can
// read regardless of owner). It is deliberately absent from usage/--help.
const ReadSocketTokenCmd = "__read-socket-token"
