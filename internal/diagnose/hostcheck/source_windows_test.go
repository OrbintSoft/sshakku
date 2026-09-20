//go:build windows

package hostcheck

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestWindowsAnswersOnlyWhatItAsked covers the two questions this build still
// does not put to the system. The distinction it keeps is between "could not
// determine" and a definite "no": the hardware key store has an answer here — a
// TPM — that nothing goes and reads, and a report saying "no" would describe a
// machine with none rather than one nobody asked. The temporary directory is
// undetermined for a different reason, there being no filesystem in memory on
// this system to find.
//
// The disk is not in this list any more; what it answers is
// TestChecksAnswerTheDiskOnThisMachine's subject, and asserting nil here would
// now be asserting the check away.
func TestWindowsAnswersOnlyWhatItAsked(t *testing.T) {
	checks := Windows{Target: `C:\Users\alice\.config\sshakku`}.Checks(t.Context())

	assert.Nil(t, checks.TmpTmpfs, "the temporary directory was not looked at")
	assert.Zero(t, checks.TmpSizeBytes, "a size nobody measured is not a size")
	assert.Nil(t, checks.SecureHardwarePresent, "nor was the hardware key store")
	assert.Empty(t, checks.SecureHardwareKind, "and nothing was found to name")
}

// TestAVolumeThatCannotBeAskedAboutIsNotReportedInTheClear. Every way the read
// can fail to happen — a path on no lettered drive, a shell that will not speak
// about it — has to arrive at the same place as a question never asked. A disk
// nobody could look at is not a disk found to be unencrypted, and the whole
// value of this check is that a reader can act on what it says.
func TestAVolumeThatCannotBeAskedAboutIsNotReportedInTheClear(t *testing.T) {
	for _, target := range []string{
		`\\server\share\keys`,
		`relative\path`,
		``,
	} {
		checks := Windows{Target: target}.Checks(t.Context())
		assert.Nilf(t, checks.DiskEncrypted,
			"nothing was asked about %q, so nothing is claimed for it", target)
	}
}

// TestNothingIsClaimedOnceTheCallerHasGivenUp. Checks takes a context and has
// to mean it: a report whose caller has stopped waiting must not go on to
// produce answers nobody is reading.
func TestNothingIsClaimedOnceTheCallerHasGivenUp(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	assert.Equal(t, Checks{}, Windows{Target: `C:\`}.Checks(ctx),
		"a cancelled look answers nothing at all")
}
