//go:build linux

package protect

// This system has no scheme this build uses to protect a key file from the
// other accounts on the machine, and says so rather than reporting the keys as
// unprotected.
//
// The difference matters: "your keys are readable by any administrator here"
// sends a reader looking for a setting to change, and on Linux the setting they
// would be looking for is not one SSHakku knows how to reach. Linux has
// candidates — fscrypt on a filesystem that supports it, a home directory on a
// volume of its own — and none of them is a file attribute that can be read and
// set the way this seam expects. Until one is implemented, nothing is claimed.
//
// See docs/HARDENING.md for what to do about it by hand in the meantime.

// Scheme names what this system protects a key with, which here is nothing.
func Scheme() string { return "" }

// Protected establishes nothing on this system.
func Protected(path string) (*bool, error) {
	_ = path
	return nil, ErrNoSchemeHere
}

// Protect does nothing on this system, and says so rather than appearing to
// have worked.
func Protect(path string) error {
	_ = path
	return ErrNoSchemeHere
}
