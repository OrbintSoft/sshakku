//go:build darwin

package main

import "github.com/OrbintSoft/sshakku/internal/diagnose"

// newHostSource returns doctor's environment-hardening HostSource for this
// OS: the FileVault/Secure Enclave reader on macOS.
func newHostSource(target string) diagnose.HostSource {
	return diagnose.DarwinHostSource{Target: target}
}

// newAncestrySource returns how the process tree is read on this OS: `ps`,
// there being no procfs to walk.
func newAncestrySource() diagnose.AncestrySource {
	return diagnose.PSAncestry{}
}

// newCgroupSource returns how a process's control-group membership is read on
// this OS, which is not at all: macOS has no control groups, and keeps no
// comparable record that would survive a double-fork and still name a launcher.
func newCgroupSource() diagnose.CgroupSource {
	return diagnose.NoCgroups{}
}
