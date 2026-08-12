//go:build darwin

package diagnose

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// settled fails the test when a check was left undetermined, and returns the
// answer it reached. nil and false are different answers — "nobody could tell"
// and "no" — and the report says different things about them, so a test that
// let one stand in for the other would agree with the wrong one.
func settled(t *testing.T, got *bool, what string) bool {
	t.Helper()
	require.NotNilf(t, got, "%s must be settled, not left undetermined", what)
	return *got
}

// stubHostProbes points the shell-out/sysctl seams at fixed results for one
// test and restores them afterward.
func stubHostProbes(t *testing.T, fdesetup func() ([]byte, error), bridge func() ([]byte, error), arm64 func() (uint32, error)) {
	t.Helper()
	of, ob, oa := fdesetupStatus, systemProfilerBridge, sysctlARM64
	t.Cleanup(func() { fdesetupStatus, systemProfilerBridge, sysctlARM64 = of, ob, oa })
	fdesetupStatus, systemProfilerBridge, sysctlARM64 = fdesetup, bridge, arm64
}

// TestDarwinChecksAppleSilicon covers Checks on an Apple Silicon host: FileVault
// On, /tmp reported not-tmpfs, and the Secure Enclave found via the arm64 sysctl
// without needing the Intel bridge probe.
func TestDarwinChecksAppleSilicon(t *testing.T) {
	stubHostProbes(t,
		func() ([]byte, error) { return []byte("FileVault is On.\n"), nil },
		func() ([]byte, error) {
			require.FailNow(t, "the Intel bridge probe must not run on an Apple Silicon host")
			return nil, nil
		},
		func() (uint32, error) { return 1, nil },
	)
	hc := DarwinHostSource{}.Checks()
	assert.True(t, settled(t, hc.DiskEncrypted, "disk encryption"), "FileVault reported on means the disk is encrypted")
	assert.False(t, settled(t, hc.TmpTmpfs, "whether /tmp is a tmpfs"), "macOS has no tmpfs on /tmp, and that is known rather than guessed")
	assert.True(t, settled(t, hc.SecureHardwarePresent, "secure hardware"), "an Apple Silicon host carries a Secure Enclave")
	assert.Equal(t, "Secure Enclave", hc.SecureHardwareKind, "what the report calls it")
}

// TestFileVaultStatusRunError covers fileVaultStatus's run-failure branch: a
// failed fdesetup yields nil (undetermined) rather than a guess.
func TestFileVaultStatusRunError(t *testing.T) {
	stubHostProbes(t,
		func() ([]byte, error) { return nil, errors.New("fdesetup missing") },
		func() ([]byte, error) { return nil, nil },
		func() (uint32, error) { return 0, errors.New("not arm64") },
	)
	assert.Nil(t, fileVaultStatus(), "a probe that could not run settles nothing, and must not guess")
}

// TestSecureEnclaveInfoIntel covers secureEnclaveInfo's Intel path: the arm64
// sysctl reports non-Apple-Silicon, so the T1/T2 bridge probe decides. It also
// covers the bridge-probe run-failure branch (undetermined).
func TestSecureEnclaveInfoIntel(t *testing.T) {
	notARM64 := func() (uint32, error) { return 0, errors.New("not arm64") }

	stubHostProbes(t, nil, func() ([]byte, error) { return []byte("Apple T2 Security Chip"), nil }, notARM64)
	present, kind := secureEnclaveInfo()
	assert.True(t, settled(t, present, "secure hardware"), "a T2 chip is secure hardware")
	assert.Equal(t, "Secure Enclave", kind, "what the report calls it")

	stubHostProbes(t, nil, func() ([]byte, error) { return []byte("no security chip"), nil }, notARM64)
	present, kind = secureEnclaveInfo()
	assert.False(t, settled(t, present, "secure hardware"), "an Intel Mac with no security chip has none")
	assert.Empty(t, kind, "there is no chip to name")

	stubHostProbes(t, nil, func() ([]byte, error) { return nil, errors.New("system_profiler failed") }, notARM64)
	present, kind = secureEnclaveInfo()
	assert.Nil(t, present, "a probe that could not run settles nothing, and must not guess")
	assert.Empty(t, kind, "nothing was learned to name")
}
