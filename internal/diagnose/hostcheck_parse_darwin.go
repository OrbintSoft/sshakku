//go:build darwin

package diagnose

import (
	"bytes"
	"strings"
)

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
