//go:build darwin

package main

import "github.com/OrbintSoft/sshakku/internal/diagnose"

// newHostSource returns doctor's environment-hardening HostSource for this
// OS: the FileVault/Secure Enclave reader on macOS.
func newHostSource(target string) diagnose.HostSource {
	return diagnose.DarwinHostSource{Target: target}
}
