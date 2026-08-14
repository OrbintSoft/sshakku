package paths

import (
	"errors"
	"io/fs"
	"os"
)

// Absent reports whether there is nothing at path at all — asked of the
// filesystem rather than read off the error some earlier call failed with.
//
// The two answers a caller has to tell apart are "you have no such directory",
// which is how most accounts start and is no error, and "what is there is not
// a directory", which is a mistake and has to be said. Systems do not agree on
// which error is which: reading a regular file as a directory fails with
// ENOTDIR on unix, but on Windows with the code for a path that is not there,
// which Go reports as both ENOTDIR *and* fs.ErrNotExist. A caller that decided
// from the error therefore reported the mistake on one platform and swallowed
// it on the other. Whether something is there is a question the filesystem
// answers the same way everywhere.
//
// Lstat, not Stat: a dangling symlink is something that is there — a link the
// user made, pointing somewhere it should not — and reporting it absent would
// hide exactly the mistake this exists to surface.
func Absent(path string) bool {
	_, err := os.Lstat(path)
	return errors.Is(err, fs.ErrNotExist)
}
