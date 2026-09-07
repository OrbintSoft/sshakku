//go:build windows

package install

import (
	"path/filepath"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

// mayWrite are the rights that let an account change what a directory holds:
// put a file in it, take one out, or hand itself more rights over it. An
// account holding any of them over the directory the machine shares can replace
// the hook every login runs.
const mayWrite = windows.GENERIC_WRITE | windows.GENERIC_ALL |
	windows.FILE_WRITE_DATA | windows.FILE_APPEND_DATA | windows.DELETE |
	windows.WRITE_DAC | windows.WRITE_OWNER

// mayRead is the right to list what a directory holds, which is the same bit as
// the right to read a file's data. Both it and the rights above are the
// specific ones and not the generic ones asked for: a list written onto an
// object has had its generic rights mapped to what they mean for that kind of
// object, so what comes back names FILE_READ_DATA where GENERIC_READ went in.
const mayRead = windows.GENERIC_READ | windows.GENERIC_ALL | windows.FILE_READ_DATA

// letEveryAccountCreateFilesIn puts on dir the permission this system's own
// shared data directory carries, so that what is created inside it is created
// under the same offer: every account may add to what this directory contains,
// and whatever they add is theirs.
//
// It is added to what is already there rather than replacing it, so the account
// running the test keeps its own access to a directory it has to clean up.
func letEveryAccountCreateFilesIn(t *testing.T, dir string) {
	t.Helper()

	everyAccount, err := windows.CreateWellKnownSid(windows.WinBuiltinUsersSid)
	require.NoError(t, err)
	descriptor, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION)
	require.NoError(t, err)
	current, _, err := descriptor.DACL()
	require.NoError(t, err)

	widened, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
		AccessPermissions: windows.GENERIC_WRITE | windows.GENERIC_READ | windows.GENERIC_EXECUTE,
		AccessMode:        windows.GRANT_ACCESS,
		Inheritance:       windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_WELL_KNOWN_GROUP,
			TrusteeValue: windows.TrusteeValueFromSID(everyAccount),
		},
	}}, current)
	require.NoError(t, err)
	require.NoError(t, windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION, nil, nil, widened, nil))
}

// accessGrantedIn is what each account may do to dir, by account, read back
// from the directory itself rather than from what was asked for.
func accessGrantedIn(t *testing.T, dir string) map[string]windows.ACCESS_MASK {
	t.Helper()

	descriptor, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION)
	require.NoError(t, err)
	list, _, err := descriptor.DACL()
	require.NoError(t, err)
	require.NotNil(t, list, "a directory with no list at all is one everybody may do anything to")

	granted := make(map[string]windows.ACCESS_MASK, list.AceCount)
	for index := range uint32(list.AceCount) {
		var entry *windows.ACCESS_ALLOWED_ACE
		require.NoError(t, windows.GetAce(list, index, &entry))
		if entry.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			continue
		}
		account := (*windows.SID)(unsafe.Pointer(&entry.SidStart))
		granted[account.String()] |= entry.Mask
	}
	return granted
}

// accountsThatMayWriteIn names every account allowed to change what dir holds.
func accountsThatMayWriteIn(t *testing.T, dir string) []string {
	t.Helper()

	writers := make([]string, 0, 2)
	for account, mask := range accessGrantedIn(t, dir) {
		if mask&mayWrite != 0 {
			writers = append(writers, account)
		}
	}
	return writers
}

// wellKnown is one of this system's own accounts, as a string to compare with.
func wellKnown(t *testing.T, which windows.WELL_KNOWN_SID_TYPE) string {
	t.Helper()

	sid, err := windows.CreateWellKnownSid(which)
	require.NoError(t, err)
	return sid.String()
}

// F59: the directory a machine-wide install shares with the whole machine is
// created writable by the system and its administrators and by nobody else —
// whatever the directory it is created inside offers, which on this system is
// the right of every account to add to what that directory contains.
func TestTheSharedDirectoryIsMadeWritableByNobodyOutsideTheMachinesOwn(t *testing.T) {
	parent := t.TempDir()
	letEveryAccountCreateFilesIn(t, parent)
	shared := filepath.Join(parent, "sshakku")

	require.NoError(t, makeDirectoryTheMachineShares(shared))

	assert.ElementsMatch(t,
		[]string{wellKnown(t, windows.WinLocalSystemSid), wellKnown(t, windows.WinBuiltinAdministratorsSid)},
		accountsThatMayWriteIn(t, shared),
		"only the system and its administrators may change what every login on the machine runs")
}

// F59: and its list says of itself that it does not come from the directory it
// sits in. That is what a person reads when every entry on it is shown without
// the mark for an inherited one, and it is what the list has to say for the
// entries above to be the whole answer rather than today's answer.
func TestTheSharedDirectorysListIsNotTheParentsToWiden(t *testing.T) {
	parent := t.TempDir()
	letEveryAccountCreateFilesIn(t, parent)
	shared := filepath.Join(parent, "sshakku")

	require.NoError(t, makeDirectoryTheMachineShares(shared))

	descriptor, err := windows.GetNamedSecurityInfo(shared, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION)
	require.NoError(t, err)
	control, _, err := descriptor.Control()
	require.NoError(t, err)
	assert.NotZero(t, control&windows.SE_DACL_PROTECTED,
		"the directory the machine shares keeps its own list, not one the directory above it may add to")
}

// F59: and every account may still read it, which is not a detail — a hook no
// login can read is a wiring that is written and never runs.
func TestTheSharedDirectoryStaysReadableByEveryAccount(t *testing.T) {
	parent := t.TempDir()
	shared := filepath.Join(parent, "sshakku")

	require.NoError(t, makeDirectoryTheMachineShares(shared))

	granted := accessGrantedIn(t, shared)
	assert.NotZero(t, granted[wellKnown(t, windows.WinBuiltinUsersSid)]&mayRead,
		"every account reads the hook its own login runs")
}

// F59: and it is the install itself that makes the directory that way. Held
// separately from the three above because they would all go on passing with
// nothing calling any of it, and a hook written into a directory made the
// ordinary way is exactly what this branch is about.
func TestAMachineWideInstallMakesTheDirectoryItSharesWritableByNobodyElse(t *testing.T) {
	hookDir := sharedDirectoryOfItsOwn(t)
	letEveryAccountCreateFilesIn(t, filepath.Dir(hookDir))
	home := t.TempDir()
	installInto(t, home)
	asASessionThatMayChangeTheMachine(t, true)

	out, err := Install(t.Context(), machineRequest(t, home, filepath.Join(home, "profile.ps1")), Ancestry{})

	require.NoError(t, err)
	assert.ElementsMatch(t,
		[]string{wellKnown(t, windows.WinLocalSystemSid), wellKnown(t, windows.WinBuiltinAdministratorsSid)},
		accountsThatMayWriteIn(t, filepath.Dir(out.HookFile)),
		"the hook an install writes goes in a directory only the machine's own may write")
}

// F59: a directory that is already there is left exactly as it was found. What
// to do about one somebody else prepared is a promise this does not yet make,
// and quietly rewriting the permissions of a directory this install did not
// create would be making it badly: the account that owns it can put them back.
func TestASharedDirectoryAlreadyThereIsLeftAsItWasFound(t *testing.T) {
	parent := t.TempDir()
	shared := filepath.Join(parent, "sshakku")
	require.NoError(t, makeDirectoryTheMachineShares(shared))
	letEveryAccountCreateFilesIn(t, shared)
	before := accessGrantedIn(t, shared)

	require.NoError(t, makeDirectoryTheMachineShares(shared))

	assert.Equal(t, before, accessGrantedIn(t, shared))
}
