//go:build windows

package handoff

import (
	"path/filepath"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

// mayEnter are the rights that let an account reach the rendezvous: list what
// the directory holds, walk through it, and open what is inside. An account
// holding any of them over the socket a passphrase is being handed across can
// connect to it and be handed the passphrase, which is the whole of the harm.
//
// Both the generic rights and the specific ones are named because a list
// written onto an object has had its generic rights mapped to what they mean
// for that kind of object: what comes back names FILE_READ_DATA where
// GENERIC_READ went in, and FILE_EXECUTE — the same bit a directory calls
// FILE_TRAVERSE — where GENERIC_EXECUTE did.
const mayEnter = windows.GENERIC_READ | windows.GENERIC_EXECUTE | windows.GENERIC_ALL |
	windows.FILE_READ_DATA | windows.FILE_EXECUTE

// accountsThatAreNobodyInParticular are this system's ways of naming somebody
// other than the account that put the passphrase aside: everybody, everybody
// who signed in, every account on the machine, and whoever is sitting at it.
// None of them may be anywhere near a rendezvous for a passphrase, and which of
// them it was does not change the answer — so they are checked as a set.
var accountsThatAreNobodyInParticular = []windows.WELL_KNOWN_SID_TYPE{
	windows.WinWorldSid,
	windows.WinAuthenticatedUserSid,
	windows.WinBuiltinUsersSid,
	windows.WinInteractiveSid,
}

// accessGrantedOver is what each account may do to path, by account, read back
// from the object itself rather than from what was asked for.
func accessGrantedOver(t *testing.T, path string) map[string]windows.ACCESS_MASK {
	t.Helper()

	descriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION)
	require.NoError(t, err)
	list, _, err := descriptor.DACL()
	require.NoError(t, err)
	require.NotNil(t, list, "an object with no list at all is one everybody may do anything to")

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

// whoeverIsNobodyInParticularMayEnter names the broad accounts above that can
// reach path, empty when none of them can.
func whoeverIsNobodyInParticularMayEnter(t *testing.T, path string) []string {
	t.Helper()

	granted := accessGrantedOver(t, path)
	reachable := make([]string, 0, len(accountsThatAreNobodyInParticular))
	for _, known := range accountsThatAreNobodyInParticular {
		account, err := windows.CreateWellKnownSid(known)
		require.NoError(t, err)
		if granted[account.String()]&mayEnter != 0 {
			reachable = append(reachable, account.String())
		}
	}
	return reachable
}

// F7: the passphrase's one resting place on this system is a socket buffer in
// the kernel, and what keeps it from everyone else is that nobody else may
// reach the socket. There is no permission bit here that says so — the ones
// this system reports for a file are synthesised from the read-only attribute
// and name nobody — so the question the other family asks of the mode is asked
// of the access-control list instead, and asked of the rendezvous the product
// really makes rather than one built for the occasion.
//
// The socket is checked as well as the directory holding it. The directory is
// what has to be walked through, but the socket is the thing that is connected
// to, and a claim that what is created inside a private directory is private
// too is a claim about the socket.
func TestNobodyButThisAccountCanReachAPassphraseInTransit(t *testing.T) {
	token, err := Stash("s3cr3t", 5*time.Second)
	require.NoError(t, err, "putting a passphrase aside must succeed")

	assert.Empty(t, whoeverIsNobodyInParticularMayEnter(t, token),
		"anything that can open this socket is handed the passphrase across it")
	assert.Empty(t, whoeverIsNobodyInParticularMayEnter(t, filepath.Dir(token)),
		"and a rendezvous is no more private than the directory it has to be reached through")

	// Collected rather than left to expire: a stash that is not taken keeps a
	// server waiting on the passphrase for the rest of its ttl, and this test is
	// done with it now.
	_, err = Fetch(t.Context(), token)
	require.NoError(t, err)
}

// The rendezvous takes its privacy from where it is put and from nothing else:
// this system offers no bit to force it with, so a directory created inside one
// that admits every account admits every account.
//
// That is why the base is the account's own cache directory, which is inside
// the profile and therefore the account's by construction, and this is the test
// that says so: give the same code a base that lets everybody in and everybody
// is let in, all the way down to the file the passphrase is served from. It is
// the other half of the test above — without it, "nobody in particular may
// enter" would read the same whether the list were narrow or unreadable.
func TestARendezvousIsOnlyEverAsPrivateAsWhereItIsPut(t *testing.T) {
	parent := t.TempDir()
	letEveryAccountIn(t, parent)

	dir, err := socketHandoffDir(parent)
	require.NoError(t, err, "making the rendezvous directory must succeed")

	everyAccount, err := windows.CreateWellKnownSid(windows.WinBuiltinUsersSid)
	require.NoError(t, err)
	assert.Contains(t, whoeverIsNobodyInParticularMayEnter(t, dir), everyAccount.String(),
		"what is created inside a directory every account may enter is one every account may enter")
}

// letEveryAccountIn puts on dir the offer a directory outside a profile can
// carry: every account on the machine may enter it and open what is inside.
//
// It is added to what is already there rather than replacing it, so the account
// running the test keeps its own access to a directory it has to clean up.
func letEveryAccountIn(t *testing.T, dir string) {
	t.Helper()

	everyAccount, err := windows.CreateWellKnownSid(windows.WinBuiltinUsersSid)
	require.NoError(t, err)
	descriptor, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION)
	require.NoError(t, err)
	current, _, err := descriptor.DACL()
	require.NoError(t, err)

	widened, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
		AccessPermissions: windows.GENERIC_READ | windows.GENERIC_EXECUTE,
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
