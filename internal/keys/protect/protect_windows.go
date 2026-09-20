//go:build windows

package protect

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// SchemeName is what this system protects a key file with: the Encrypting File
// System, which encrypts the file to the account that owns it. Another account
// on the machine, administrator or not, acting as itself, cannot read it; nor
// can anyone holding the disk while that account is not signed in.
//
// What it is not: protection from an administrator who can run as the system
// while the owner is signed in, since that can borrow the owner's own identity;
// and not protection from a recovery agent, which an organisation configures
// precisely so that it can decrypt.
const SchemeName = "EFS"

var (
	advapi32         = windows.NewLazySystemDLL("advapi32.dll")
	procEncryptFileW = advapi32.NewProc("EncryptFileW")
)

// Scheme names what this system protects a key with.
func Scheme() string { return SchemeName }

// Protected reports whether the file or directory at path is encrypted to an
// account, or nil where that could not be told.
func Protected(path string) (*bool, error) {
	wide, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, fmt.Errorf("read the attributes of %s: %w", path, err)
	}
	attrs, err := windows.GetFileAttributes(wide)
	if err != nil {
		return nil, fmt.Errorf("read the attributes of %s: %w", path, err)
	}
	encrypted := attrs&windows.FILE_ATTRIBUTE_ENCRYPTED != 0
	return &encrypted, nil
}

// Protect encrypts the file or directory at path to the account running this.
//
// On a directory this covers the files created in it afterwards and not the
// ones already there, which is why the caller hands the keys over one by one as
// well. The call is documented as returning success without having done
// anything where a directory holds a read-only file, so what it returns is
// never taken as evidence: the caller reads the path back.
func Protect(path string) error {
	wide, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return fmt.Errorf("protect %s: %w", path, err)
	}
	ok, _, callErr := procEncryptFileW.Call(uintptr(unsafe.Pointer(wide)))
	if ok == 0 {
		return fmt.Errorf("protect %s: %w", path, callErr)
	}
	return nil
}
