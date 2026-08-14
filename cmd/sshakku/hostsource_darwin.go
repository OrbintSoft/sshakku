//go:build darwin

package main

import (
	"github.com/OrbintSoft/sshakku/internal/diagnose/hostcheck"
	"github.com/OrbintSoft/sshakku/internal/diagnose/launcher"
)

// newHostSource returns doctor's environment-hardening source for this
// OS: the FileVault/Secure Enclave reader on macOS.
func newHostSource(target string) hostcheck.Source {
	return hostcheck.Darwin{Target: target}
}

// newAncestrySource returns how the process tree is read on this OS: `ps`,
// there being no procfs to walk.
func newAncestrySource() launcher.AncestrySource {
	return launcher.PSAncestry{}
}

// newCgroupSource returns how a process's control-group membership is read on
// this OS, which is not at all: macOS has no control groups, and keeps no
// comparable record that would survive a double-fork and still name a launcher.
func newCgroupSource() launcher.CgroupSource {
	return launcher.NoCgroups{}
}
