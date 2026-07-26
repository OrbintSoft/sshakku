//go:build darwin

package diagnose

import (
	"context"
	"os/exec"
	"time"

	"golang.org/x/sys/unix"
)

// hostCheckTimeout bounds each of DarwinHostSource's shell-outs. These are
// plain status queries, not human-facing prompts, so a wedged binary (or an
// agent/authorization dependency it silently blocks on) must surface as an
// undetermined check, not hang the caller (e.g. `doctor`) indefinitely.
const hostCheckTimeout = 5 * time.Second

// Seams over the macOS host-status probes — each a shell-out or sysctl side
// effect — so DarwinHostSource.Checks and its helpers' branch logic are
// unit-testable by stubbing them. Production points them at the real tools.
var (
	fdesetupStatus = func() ([]byte, error) {
		//coverage:ignore
		return hostCheckOutput("fdesetup", "status")
	}
	systemProfilerBridge = func() ([]byte, error) {
		//coverage:ignore
		return hostCheckOutput("system_profiler", "SPiBridgeDataType")
	}
	sysctlARM64 = func() (uint32, error) {
		//coverage:ignore
		return unix.SysctlUint32("hw.optional.arm64")
	}
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
// to query (only to change), and interprets its output. nil on any output the
// parser doesn't recognize (or a run failure), rather than guessing.
func fileVaultStatus() *bool {
	out, err := fdesetupStatus()
	if err != nil {
		return nil
	}
	return parseFileVaultStatus(out)
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
	if arm64, err := sysctlARM64(); err == nil && arm64 == 1 {
		present := true
		return &present, "Secure Enclave"
	}
	out, err := systemProfilerBridge()
	if err != nil {
		return nil, ""
	}
	return bridgeSecureEnclave(out)
}

// hostCheckOutput runs a status probe with hostCheckTimeout so a wedged binary
// surfaces as an undetermined check rather than hanging the caller.
func hostCheckOutput(name string, args ...string) ([]byte, error) {
	//coverage:ignore
	ctx, cancel := context.WithTimeout(context.Background(), hostCheckTimeout)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).Output()
}
