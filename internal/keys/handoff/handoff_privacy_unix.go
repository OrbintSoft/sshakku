//go:build unix

package handoff

import "os"

// What makes a handoff rendezvous private here: the permission bits on the
// directory and on the socket in it. They are set rather than inherited,
// because a directory that already existed may have been made with looser ones,
// and what is being guarded is a passphrase.
//
// They are seams so the permission-failure branches are exercisable without a
// filesystem that refuses.
var (
	chmodDir  = os.Chmod
	chmodSock = os.Chmod
)
