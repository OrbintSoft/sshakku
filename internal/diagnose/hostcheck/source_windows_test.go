//go:build windows

package hostcheck

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestWindowsChecksAreUndeterminedRatherThanNegative covers the whole of what
// this build observes about the host: nothing. The distinction it has to keep is
// between "could not determine" and a definite "no" — the questions have answers
// on this system (BitLocker for the disk, a TPM for the hardware key store) and
// this build asks neither, so a report that said "no" would describe a machine
// with no protection at all rather than one nobody looked at.
func TestWindowsChecksAreUndeterminedRatherThanNegative(t *testing.T) {
	checks := Windows{Target: `C:\Users\alice\.config\sshakku`}.Checks(t.Context())

	assert.Nil(t, checks.DiskEncrypted, "the disk was not looked at, so it is not answered for")
	assert.Nil(t, checks.TmpTmpfs, "nor was the temporary directory")
	assert.Zero(t, checks.TmpSizeBytes, "a size nobody measured is not a size")
	assert.Nil(t, checks.SecureHardwarePresent, "nor was the hardware key store")
	assert.Empty(t, checks.SecureHardwareKind, "and nothing was found to name")
}
