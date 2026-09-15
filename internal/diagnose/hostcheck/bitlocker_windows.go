//go:build windows

package hostcheck

// bitLockerProtection turns what the shell says about a volume into the
// report's three answers.
//
// Nothing here is implemented yet; the tests beside it say what it has to do.
func bitLockerProtection(value int32) *bool {
	_ = value
	return nil
}

// volumeRootOf returns the root of the lettered volume a path is on, and
// whether there was one.
//
// Nothing here is implemented yet; the tests beside it say what it has to do.
func volumeRootOf(target string) (string, bool) {
	_ = target
	return "", false
}

// volumeProtection asks the shell what BitLocker is doing with the volume at
// root.
//
// Nothing here is implemented yet; the tests beside it say what it has to do.
func volumeProtection(root string) (*bool, error) {
	_ = root
	return nil, nil
}
