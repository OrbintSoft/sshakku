//go:build darwin

package protect

// This system has no scheme this build uses to protect a key file from the
// other accounts on the machine, and says so rather than reporting the keys as
// unprotected.
//
// The difference matters: "your keys are readable by any administrator here"
// sends a reader looking for a setting to change, and on macOS there is no
// per-file one to find. FileVault encrypts the whole volume and is answered by
// the disk-encryption check instead; it protects a Mac that is off, not one
// account from another on a Mac that is running. The nearest per-directory
// equivalent is an encrypted disk image, which is a thing somebody sets up and
// mounts rather than an attribute on a file.
//
// See docs/HARDENING.md for what that means in practice.

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
