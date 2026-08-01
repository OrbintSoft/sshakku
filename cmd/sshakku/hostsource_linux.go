//go:build linux

package main

import "github.com/OrbintSoft/sshakku/internal/diagnose"

// newHostSource returns doctor's environment-hardening HostSource for this
// OS: the real /proc, /sys, /dev reader on Linux.
func newHostSource(target string) diagnose.HostSource {
	return diagnose.ProcfsHostSource{Target: target}
}

// newAncestrySource returns how the process tree is read on this OS: the
// procfs walk on Linux, which is what lets doctor name whatever launched an
// agent.
func newAncestrySource() diagnose.AncestrySource {
	return diagnose.ProcfsAncestry{}
}

// newCgroupSource returns how a process's control-group membership is read on
// this OS. It is the fallback for an agent that double-forked, where the
// process tree can no longer say who launched it but the cgroup still names the
// systemd unit.
func newCgroupSource() diagnose.CgroupSource {
	return diagnose.ProcfsCgroup{}
}
