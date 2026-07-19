//go:build linux

package main

import "github.com/OrbintSoft/sshakku/internal/diagnose"

// newHostSource returns doctor's environment-hardening HostSource for this
// OS: the real /proc, /sys, /dev reader on Linux.
func newHostSource(target string) diagnose.HostSource {
	return diagnose.ProcfsHostSource{Target: target}
}
