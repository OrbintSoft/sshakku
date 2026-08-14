//go:build linux

package main

import (
	"github.com/OrbintSoft/sshakku/internal/diagnose/hostcheck"
	"github.com/OrbintSoft/sshakku/internal/diagnose/launcher"
)

// newHostSource returns doctor's environment-hardening source for this
// OS: the real /proc, /sys, /dev reader on Linux.
func newHostSource(target string) hostcheck.Source {
	return hostcheck.Procfs{Target: target}
}

// newAncestrySource returns how the process tree is read on this OS: the
// procfs walk on Linux, which is what lets doctor name whatever launched an
// agent.
func newAncestrySource() launcher.AncestrySource {
	return launcher.ProcfsAncestry{}
}

// newCgroupSource returns how a process's control-group membership is read on
// this OS. It is the fallback for an agent that double-forked, where the
// process tree can no longer say who launched it but the cgroup still names the
// systemd unit.
func newCgroupSource() launcher.CgroupSource {
	return launcher.ProcfsCgroup{}
}
