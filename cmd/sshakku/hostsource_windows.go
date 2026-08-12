//go:build windows

package main

import "github.com/OrbintSoft/sshakku/internal/diagnose"

// newHostSource returns doctor's environment-hardening HostSource for this OS,
// which determines nothing here — see diagnose.WindowsHostSource.
func newHostSource(target string) diagnose.HostSource {
	return diagnose.WindowsHostSource{Target: target}
}

// newAncestrySource returns how the process tree is read on this OS, which is
// not at all: there is no procfs to walk and no `ps` to ask.
func newAncestrySource() diagnose.AncestrySource {
	return diagnose.NoAncestry{}
}

// newCgroupSource returns how a process's control-group membership is read on
// this OS, which is not at all: Windows has no control groups.
func newCgroupSource() diagnose.CgroupSource {
	return diagnose.NoCgroups{}
}
