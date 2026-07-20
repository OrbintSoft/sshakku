//go:build darwin

package diagnose

import (
	"bytes"
	"os/exec"
	"strings"

	"golang.org/x/sys/unix"
)

// DarwinHostSource gathers HostChecks via macOS-native tools: `fdesetup
// status` for FileVault, and CPU architecture (falling back to a T1/T2 probe
// on Intel) for Secure Enclave presence. Target is unused — FileVault status
// is whole-volume, unlike Linux's per-mount LUKS check — kept only for
// interface parity with ProcfsHostSource.
type DarwinHostSource struct {
	Target string
}

// Checks implements HostSource.
func (DarwinHostSource) Checks() HostChecks {
	var hc HostChecks
	hc.DiskEncrypted = fileVaultStatus()
	notTmpfs := false
	hc.TmpTmpfs = &notTmpfs // macOS has no tmpfs-backed /tmp to detect
	hc.SecureHardwarePresent, hc.SecureHardwareKind = secureEnclaveInfo()
	return hc
}

// fileVaultStatus runs `fdesetup status`, which needs no elevated privilege
// to query (only to change). nil on any output this parser doesn't
// recognize, rather than guessing.
func fileVaultStatus() *bool {
	out, err := exec.Command("fdesetup", "status").Output()
	if err != nil {
		return nil
	}
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

// secureEnclaveInfo reports whether the machine has a Secure Enclave
// Processor. Every Apple Silicon Mac has one built into the SoC — no probe
// needed beyond CPU architecture, read via the same "hw.optional.arm64"
// sysctl the OS itself uses to decide whether Rosetta is needed. This
// reflects the *host*, unlike checking the running binary's own GOARCH,
// which would misreport under Rosetta 2 emulation. Only Intel Macs need an
// actual probe, since a Secure Enclave there was optional, tied to a T1/T2
// Security Chip; `system_profiler`'s bridge/coprocessor data type names it
// directly when present. Deliberately avoids IOKit registry class names
// (e.g. `ioreg -c <class>`) for this: they are internal implementation
// details Apple can rename between OS releases, unlike a public sysctl name.
func secureEnclaveInfo() (*bool, string) {
	if arm64, err := unix.SysctlUint32("hw.optional.arm64"); err == nil && arm64 == 1 {
		present := true
		return &present, "Secure Enclave"
	}
	out, err := exec.Command("system_profiler", "SPiBridgeDataType").Output()
	if err != nil {
		return nil, ""
	}
	present := bytes.Contains(out, []byte("Apple T1")) || bytes.Contains(out, []byte("Apple T2"))
	if present {
		return &present, "Secure Enclave"
	}
	return &present, ""
}
