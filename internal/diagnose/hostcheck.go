package diagnose

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
