//go:build darwin

package diagnose

import (
	"errors"
	"testing"
)

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
		func() ([]byte, error) { t.Fatal("bridge probe must not run on arm64"); return nil, nil },
		func() (uint32, error) { return 1, nil },
	)
	hc := DarwinHostSource{}.Checks()
	if hc.DiskEncrypted == nil || !*hc.DiskEncrypted {
		t.Errorf("DiskEncrypted = %v, want true", hc.DiskEncrypted)
	}
	if hc.TmpTmpfs == nil || *hc.TmpTmpfs {
		t.Errorf("TmpTmpfs = %v, want a definite false", hc.TmpTmpfs)
	}
	if hc.SecureHardwarePresent == nil || !*hc.SecureHardwarePresent || hc.SecureHardwareKind != "Secure Enclave" {
		t.Errorf("SecureHardware = (%v, %q), want (true, Secure Enclave)", hc.SecureHardwarePresent, hc.SecureHardwareKind)
	}
}

// TestFileVaultStatusRunError covers fileVaultStatus's run-failure branch: a
// failed fdesetup yields nil (undetermined) rather than a guess.
func TestFileVaultStatusRunError(t *testing.T) {
	stubHostProbes(t,
		func() ([]byte, error) { return nil, errors.New("fdesetup missing") },
		func() ([]byte, error) { return nil, nil },
		func() (uint32, error) { return 0, errors.New("not arm64") },
	)
	if got := fileVaultStatus(); got != nil {
		t.Errorf("fileVaultStatus() = %v, want nil on a run failure", *got)
	}
}

// TestSecureEnclaveInfoIntel covers secureEnclaveInfo's Intel path: the arm64
// sysctl reports non-Apple-Silicon, so the T1/T2 bridge probe decides. It also
// covers the bridge-probe run-failure branch (undetermined).
func TestSecureEnclaveInfoIntel(t *testing.T) {
	notARM64 := func() (uint32, error) { return 0, errors.New("not arm64") }

	stubHostProbes(t, nil, func() ([]byte, error) { return []byte("Apple T2 Security Chip"), nil }, notARM64)
	if present, kind := secureEnclaveInfo(); present == nil || !*present || kind != "Secure Enclave" {
		t.Errorf("secureEnclaveInfo() = (%v, %q), want (true, Secure Enclave)", present, kind)
	}

	stubHostProbes(t, nil, func() ([]byte, error) { return []byte("no security chip"), nil }, notARM64)
	if present, kind := secureEnclaveInfo(); present == nil || *present || kind != "" {
		t.Errorf("secureEnclaveInfo() = (%v, %q), want (false, \"\")", present, kind)
	}

	stubHostProbes(t, nil, func() ([]byte, error) { return nil, errors.New("system_profiler failed") }, notARM64)
	if present, kind := secureEnclaveInfo(); present != nil || kind != "" {
		t.Errorf("secureEnclaveInfo() = (%v, %q), want (nil, \"\") on a probe failure", present, kind)
	}
}
