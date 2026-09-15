//go:build windows

package hostcheck

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Verifies F68 on this platform. The question the report asks is whether the
// disk holding the keys is encrypted, and until now this build answered
// "could not tell" to it on every Windows machine.

// TestOnlyProtectedReadsAsProtected is the whole of the safety in this
// mapping. The shell tells us what BitLocker is doing as a number, and
// Microsoft documents the property's type without documenting what its values
// mean — so the one value taken to mean "protected" is taken deliberately and
// every other known value is read the other way. A volume halfway through
// being encrypted is not an encrypted volume, which is the same line Microsoft
// draws for the status it does document: partially encrypted is PROTECTION
// OFF.
func TestOnlyProtectedReadsAsProtected(t *testing.T) {
	yes := bitLockerProtection(1)
	require.NotNil(t, yes, "the protected value has to be an answer, not a shrug")
	assert.True(t, *yes, "and it has to be yes")

	for _, value := range []int32{0, 2, 3, 4, 5, 6} {
		answer := bitLockerProtection(value)
		require.NotNilf(t, answer, "%d is a value with a meaning, so it is answered", value)
		assert.Falsef(t, *answer, "%d is not a volume whose key is out of reach", value)
	}
}

// TestAValueThisBuildDoesNotKnowIsNotAnAnswer. The values are not documented,
// so a number outside the set above is a number whose meaning this build does
// not have. Reported as "not encrypted" it would send somebody to turn on
// something already on; reported as "encrypted" it would tell them their keys
// are covered when nothing says so. Neither is worth doing to be able to print
// a line.
func TestAValueThisBuildDoesNotKnowIsNotAnAnswer(t *testing.T) {
	for _, value := range []int32{-1, 7, 99} {
		assert.Nilf(t, bitLockerProtection(value),
			"%d means nothing to this build, and a report says so rather than guessing", value)
	}
}

// TestTheVolumeAskedAboutIsTheOneHoldingTheTarget: the question is about the
// disk the keys are on, which on a machine with more than one drive is not
// whichever one the system happens to boot from.
func TestTheVolumeAskedAboutIsTheOneHoldingTheTarget(t *testing.T) {
	for target, want := range map[string]string{
		`C:\Users\alice\.ssh`:            `C:\`,
		`D:\keys`:                        `D:\`,
		`c:\users\alice\.config\sshakku`: `c:\`,
	} {
		got, ok := volumeRootOf(target)
		require.Truef(t, ok, "a path on a lettered drive names a volume: %q", target)
		assert.Equal(t, want, got, target)
	}
}

// TestAPathOnNoLetteredVolumeIsNotAskedAbout. A network share has no drive
// letter to hand the shell, and the question of whether "the disk" is
// encrypted is not one this can answer for somebody else's server.
func TestAPathOnNoLetteredVolumeIsNotAskedAbout(t *testing.T) {
	for _, target := range []string{
		`\\server\share\keys`,
		`\\?\Volume{00000000-0000-0000-0000-000000000000}\keys`,
		`relative\path`,
		``,
	} {
		_, ok := volumeRootOf(target)
		assert.Falsef(t, ok, "nothing lettered to ask about: %q", target)
	}
}

// TestChecksAnswerTheDiskOnThisMachine drives the real shell on whatever
// machine this runs on. It asserts the shape rather than the value — a runner's
// disk may be encrypted or not, and a test that demanded either would be
// testing the runner — but a nil here means the read did not work at all,
// which is the failure this exists to catch.
func TestChecksAnswerTheDiskOnThisMachine(t *testing.T) {
	checks := Windows{Target: `C:\`}.Checks(t.Context())

	require.NotNil(t, checks.DiskEncrypted,
		"the disk on this machine has to be answered for, either way")
}
