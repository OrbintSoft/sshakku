package main

// TargetTokenSource reads another user's per-login socket token. A kernel
// keyring is only visible to the uid that owns it, unlike files, which root can
// read regardless of owner, so an implementation must itself assume the target
// uid's identity to succeed — that privilege transition is ReadToken's
// responsibility, not its caller's.
type TargetTokenSource interface {
	// ReadToken returns the target uid/gid's socket token, or "" when none
	// exists yet (a valid, tokenless state, not an error).
	ReadToken(uid, gid int) (string, error)
}
