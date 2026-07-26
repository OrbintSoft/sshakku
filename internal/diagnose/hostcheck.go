package diagnose

import (
	"bytes"
	"strings"
)

// HostChecks are best-effort, read-only observations about the host
// environment: conditions outside sshakku's own control that materially
// affect its threat model (a leaked wallet on an unencrypted disk, a
// passphrase transiting a world-readable /tmp). A nil pointer means "could
// not determine" — these checks never guess, and doctor only ever reports
// them, never configures or refuses to run because of them.
type HostChecks struct {
	// DiskEncrypted reports whether the disk backing Target is encrypted:
	// LUKS (including one level of LUKS-under-LVM) on Linux, FileVault on
	// macOS. nil when that could not be resolved (network filesystem,
	// overlay, tmpfs root, missing /proc/mounts, an unparseable `fdesetup
	// status`).
	DiskEncrypted *bool

	// TmpTmpfs reports whether /tmp is its own tmpfs mount, as opposed to
	// living on the root filesystem. nil when this could not be determined.
	TmpTmpfs *bool
	// TmpSizeBytes is /tmp's total size when TmpTmpfs is true; 0 when
	// TmpTmpfs is not true, or the size could not be determined.
	TmpSizeBytes int64

	// SecureHardwarePresent reports whether the machine has a hardware key
	// store an OS-level encryption scheme could bind to: a TPM on Linux, the
	// Secure Enclave on macOS. nil when this could not be determined; a
	// definite "no" is itself a determination, not an unknown.
	SecureHardwarePresent *bool
	// SecureHardwareKind names what SecureHardwarePresent found — "TPM 2.0",
	// "TPM 1.2", or "Secure Enclave" — empty when not present or undetermined.
	SecureHardwareKind string
}

// HostSource gathers HostChecks. ProcfsHostSource (Linux) and
// DarwinHostSource (macOS) are the real implementations. Tests supply a
// fake.
type HostSource interface {
	Checks() HostChecks
}

// parseFileVaultStatus interprets the output of macOS's `fdesetup status`: a
// definite On/Off, or nil for anything this parser doesn't recognize (so the
// check reports "undetermined" rather than guessing). Kept here, separate from
// the shell-out in hostcheck_darwin.go, so it is unit-testable without running
// fdesetup.
func parseFileVaultStatus(out []byte) *bool {
	switch s := strings.TrimSpace(string(out)); {
	case strings.HasPrefix(s, "FileVault is On"):
		on := true
		return &on
	case strings.HasPrefix(s, "FileVault is Off"):
		off := false
		return &off
	default:
		return nil
	}
}

// bridgeSecureEnclave interprets the output of macOS's `system_profiler
// SPiBridgeDataType` on Intel Macs: an Apple T1/T2 Security Chip carries a
// Secure Enclave. It returns a definite present/absent (never nil — a run that
// produced output is itself a determination) and the hardware kind for a
// present one. Kept here, separate from the shell-out, so it is unit-testable
// without running system_profiler.
func bridgeSecureEnclave(out []byte) (*bool, string) {
	present := bytes.Contains(out, []byte("Apple T1")) || bytes.Contains(out, []byte("Apple T2"))
	if present {
		return &present, "Secure Enclave"
	}
	return &present, ""
}
