//go:build windows

package move

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// Permit gives one path a list naming this account and nobody else, marked as
// not the parent's to add to.
//
// This is what OpenSSH on this system asks of a private key: it refuses to use
// one that another account can reach, and says so at the moment somebody is
// trying to connect. Inheritance is cut rather than added to, because a home
// directory hands what is under it entries this account never chose, and an
// entry left in place is an account left able to read the key.
//
// The account is not named in the list twice over as owner: a file this account
// moved is already owned by it, and taking ownership of one that is not is a
// larger act than moving a key, and a different promise.
func Permit(path string, kind Kind) error {
	me, err := thisAccount()
	if err != nil {
		return err
	}

	// A directory passes its list on, so a key generated in it tomorrow is born
	// with the same one. A file has nothing under it to pass anything to.
	inheritance := uint32(windows.NO_INHERITANCE)
	if kind == Directory {
		inheritance = windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT
	}

	list, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
		AccessPermissions: windows.GENERIC_ALL,
		AccessMode:        windows.GRANT_ACCESS,
		Inheritance:       inheritance,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_USER,
			TrusteeValue: windows.TrusteeValueFromSID(me),
		},
	}}, nil)
	if err != nil {
		// Unreachable: the one entry is built here from an account this system
		// has just named, and a list is refused only when the system cannot
		// allocate one.
		//coverage:ignore
		return fmt.Errorf("building the permissions of %s: %w", path, err)
	}

	err = windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, list, nil)
	if err != nil {
		return fmt.Errorf("permitting %s: %w", path, err)
	}
	return nil
}

// thisAccount is the account running this, as a SID.
func thisAccount() (*windows.SID, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		//coverage:ignore
		return nil, fmt.Errorf("naming the account these keys belong to: %w", err)
	}
	return user.User.Sid, nil
}
