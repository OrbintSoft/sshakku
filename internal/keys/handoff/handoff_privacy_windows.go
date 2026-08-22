//go:build windows

package handoff

import "io/fs"

// What makes a handoff rendezvous private here is where it is, not a bit on it.
// The socket goes under this account's own profile, whose access-control list
// names the account, and everything created inside inherits it — so the
// directory and the socket are the account's from the moment they exist, with
// no window in which they are not.
//
// The permission bits this system reports for a file are synthesised from the
// read-only attribute and say nothing about who may enter, so setting them
// would be a gesture rather than a guard: it would make this code look like the
// other family's without doing what that family's bits do. Nothing is set here,
// and the sentence above is the reason.
//
// They are still seams, so a test can make the step fail and see what the stash
// does about it.
var (
	chmodDir  = inheritedFromTheProfile
	chmodSock = inheritedFromTheProfile
)

// inheritedFromTheProfile is the privacy step on a system where privacy comes
// from the directory a thing is created in.
func inheritedFromTheProfile(string, fs.FileMode) error { return nil }
