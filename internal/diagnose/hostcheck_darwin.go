//go:build darwin

package diagnose

import (
	"bytes"
	"os/exec"
	"strings"
)

// DarwinHostSource gathers HostChecks via macOS-native tools: `fdesetup
// status` for FileVault, and an AppleSEPManager IOKit-registry probe for
// Secure Enclave presence. Target is unused — FileVault status is
// whole-volume, unlike Linux's per-mount LUKS check — kept only for
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
// Processor, via the same AppleSEPManager IOKit registry entry present on
// every Mac with one — T1/T2 Intel Macs and all Apple Silicon Macs alike, so
// this needs no separate check per CPU architecture. nil when `ioreg` itself
// could not run; a clean run with no match is a real "absent" determination.
func secureEnclaveInfo() (*bool, string) {
	out, err := exec.Command("ioreg", "-c", "AppleSEPManager", "-d", "1").Output()
	if err != nil {
		return nil, ""
	}
	present := bytes.Contains(out, []byte("AppleSEPManager"))
	if present {
		return &present, "Secure Enclave"
	}
	return &present, ""
}
