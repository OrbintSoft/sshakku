//go:build windows

package install

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

// makeDirectoryTheMachineShares makes the directory a machine-wide install's
// hook goes in, permitted the way a directory the whole machine reads has to
// be: every account may read what is in it, because every account's login runs
// the hook, and none but the system and its administrators may write there.
//
// The permissions are given at creation and not set afterwards, because the
// gap between the two is the whole of the problem. This system's shared data
// directory hands every account the right to create files and directories in
// what it contains, and hands whoever creates one full control of it, so a
// directory made the ordinary way is one any account may add to — and one any
// account may own outright, by making it before anybody installs. What lives in
// it is the file every login on the machine runs, administrators' included.
//
// A directory already there is left exactly as it was found, and this is
// deliberate: what to do about one somebody else prepared is a different
// promise from this one. Rewriting the permissions of a directory this install
// did not create would look like an answer without being one — the account that
// owns it can put them back — and answering it properly belongs to an installer
// running as the system itself.
func makeDirectoryTheMachineShares(path string) error {
	// The parent, the ordinary way: it is this system's own directory, made
	// long before this ran, and its permissions are not ours to state.
	if err := os.MkdirAll(filepath.Dir(path), hookDirMode); err != nil {
		return fmt.Errorf("making %s: %w", filepath.Dir(path), err)
	}

	permissions, err := writableOnlyByTheMachinesOwn()
	if err != nil {
		return err
	}
	wide, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return fmt.Errorf("making %s: %w", path, err)
	}
	err = windows.CreateDirectory(wide, permissions)
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return fmt.Errorf("making %s: %w", path, err)
	}
	return nil
}

// whoMayDoWhat is the machine's own answer about a directory it shares: who is
// named in it, and what each of them may do.
//
// Administrators are named as well as the system because a machine-wide install
// is run by one of them, and a directory its own installer could not write to
// could never be installed into twice. Every account is named too, with reading
// and no more: a hook no login can read is a wiring that is written and never
// runs.
var whoMayDoWhat = []struct {
	account windows.WELL_KNOWN_SID_TYPE
	may     windows.ACCESS_MASK
}{
	{windows.WinLocalSystemSid, windows.GENERIC_ALL},
	{windows.WinBuiltinAdministratorsSid, windows.GENERIC_ALL},
	{windows.WinBuiltinUsersSid, windows.GENERIC_READ | windows.GENERIC_EXECUTE},
}

// writableOnlyByTheMachinesOwn builds what a directory is created with, from
// the table above.
//
// Stating a list at creation is itself what keeps the directory above out of
// it: a directory made with one of its own is given that list and nothing else,
// and the list is marked as not the parent's to add to — which is what a person
// reads when `icacls` shows every entry without the mark for an inherited one.
// A directory made the ordinary way and permitted afterwards would be two
// steps, and the first of them would already have offered every account the
// right to create files in it.
func writableOnlyByTheMachinesOwn() (*windows.SecurityAttributes, error) {
	entries := make([]windows.EXPLICIT_ACCESS, 0, len(whoMayDoWhat))
	for _, who := range whoMayDoWhat {
		account, err := windows.CreateWellKnownSid(who.account)
		if err != nil {
			return nil, fmt.Errorf("naming the accounts a shared directory is for: %w", err)
		}
		entries = append(entries, windows.EXPLICIT_ACCESS{
			AccessPermissions: who.may,
			AccessMode:        windows.GRANT_ACCESS,
			Inheritance:       windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT,
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeType:  windows.TRUSTEE_IS_WELL_KNOWN_GROUP,
				TrusteeValue: windows.TrusteeValueFromSID(account),
			},
		})
	}

	list, err := windows.ACLFromEntries(entries, nil)
	if err != nil {
		return nil, fmt.Errorf("building the permissions of a shared directory: %w", err)
	}
	permissions, err := windows.NewSecurityDescriptor()
	if err != nil {
		return nil, fmt.Errorf("building the permissions of a shared directory: %w", err)
	}
	if err := permissions.SetDACL(list, true, false); err != nil {
		return nil, fmt.Errorf("building the permissions of a shared directory: %w", err)
	}
	return &windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		SecurityDescriptor: permissions,
	}, nil
}
