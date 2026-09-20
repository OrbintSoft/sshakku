//go:build windows

package move

import (
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

// Verifies F70 against real permissions rather than a stand-in. What a key file
// must not be is readable by another account, and the only thing that can say
// whether it is, is the list the filesystem holds.

// letEveryAccountReadIt adds to path's list the entry this exists to remove: a
// key another account may read. It is added to what is already there, so the
// account running the test keeps the access it needs to clean up.
func letEveryAccountReadIt(t *testing.T, path string) {
	t.Helper()

	everyAccount, err := windows.CreateWellKnownSid(windows.WinBuiltinUsersSid)
	require.NoError(t, err)
	descriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION)
	require.NoError(t, err)
	current, _, err := descriptor.DACL()
	require.NoError(t, err)

	widened, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
		AccessPermissions: windows.GENERIC_READ,
		AccessMode:        windows.GRANT_ACCESS,
		Inheritance:       windows.NO_INHERITANCE,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_WELL_KNOWN_GROUP,
			TrusteeValue: windows.TrusteeValueFromSID(everyAccount),
		},
	}}, current)
	require.NoError(t, err)
	require.NoError(t, windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION, nil, nil, widened, nil))
}

// accountsNamedOn is every account the filesystem says has an entry on path,
// read back from the file itself rather than from what was asked for.
func accountsNamedOn(t *testing.T, path string) []string {
	t.Helper()

	descriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION)
	require.NoError(t, err)
	list, _, err := descriptor.DACL()
	require.NoError(t, err)
	require.NotNil(t, list, "a file with no list at all is one anybody may read")

	var named []string
	for index := range uint32(list.AceCount) {
		var entry *windows.ACCESS_ALLOWED_ACE
		require.NoError(t, windows.GetAce(list, index, &entry))
		account := (*windows.SID)(unsafe.Pointer(&entry.SidStart))
		named = append(named, account.String())
	}
	return named
}

func meHere(t *testing.T) string {
	t.Helper()
	me, err := thisAccount()
	require.NoError(t, err)
	return me.String()
}

// TestAKeyEndsUpNamingThisAccountAndNobodyElse. OpenSSH here refuses a private
// key another account can reach, and says so at the moment somebody is trying
// to connect rather than when the key was put there.
func TestAKeyEndsUpNamingThisAccountAndNobodyElse(t *testing.T) {
	key := filepath.Join(t.TempDir(), "id_ed25519")
	require.NoError(t, os.WriteFile(key, []byte("not really a key"), 0o600))
	letEveryAccountReadIt(t, key)
	everyAccount, err := windows.CreateWellKnownSid(windows.WinBuiltinUsersSid)
	require.NoError(t, err)
	require.Containsf(t, accountsNamedOn(t, key), everyAccount.String(),
		"the precondition: every account can read it to begin with, or this test proves nothing")

	require.NoError(t, Permit(key, PrivateKey))

	assert.Equal(t, []string{meHere(t)}, accountsNamedOn(t, key),
		"one account is named afterwards, and it is this one")
}

// TestADirectoryPassesItsListToWhatIsMadeInIt is what keeps a key generated
// there tomorrow as private as the ones moved there today.
func TestADirectoryPassesItsListToWhatIsMadeInIt(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "keys")
	require.NoError(t, os.Mkdir(dir, 0o700))

	require.NoError(t, Permit(dir, Directory))

	born := filepath.Join(dir, "id_later")
	require.NoError(t, os.WriteFile(born, []byte("not really a key"), 0o600))
	assert.Equal(t, []string{meHere(t)}, accountsNamedOn(t, born),
		"a key made inside it is born naming this account and nobody else")
}

// TestWhatThisSystemInheritsIsCutOffRatherThanAddedTo. A home directory hands
// what is under it entries this account never chose, and an entry left in place
// is an account left able to read the key.
func TestWhatThisSystemInheritsIsCutOffRatherThanAddedTo(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "keys")
	require.NoError(t, os.Mkdir(dir, 0o700))
	require.NoError(t, Permit(dir, Directory))

	descriptor, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION)
	require.NoError(t, err)
	control, _, err := descriptor.Control()
	require.NoError(t, err)

	assert.NotZerof(t, control&windows.SE_DACL_PROTECTED,
		"the list is marked as not the parent's to add to; control=%#x", control)
}

// TestAPathThatIsNotThereIsARefusalRatherThanASilence, so a run that could not
// permit a key stops instead of reporting that it did.
func TestAPathThatIsNotThereIsARefusalRatherThanASilence(t *testing.T) {
	err := Permit(filepath.Join(t.TempDir(), "nothing-here"), PrivateKey)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "nothing-here", "and names what it could not do")
}
