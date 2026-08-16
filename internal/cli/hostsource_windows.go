//go:build windows

package cli

import (
	"github.com/OrbintSoft/sshakku/internal/diagnose/hostcheck"
	"github.com/OrbintSoft/sshakku/internal/diagnose/launcher"
)

// newHostSource returns doctor's environment-hardening source for this OS,
// which determines nothing here — see hostcheck.Windows.
func newHostSource(target string) hostcheck.Source {
	return hostcheck.Windows{Target: target}
}

// newAncestrySource returns how the process tree is read on this OS: a
// Toolhelp32 snapshot, there being no procfs to walk and no `ps` to ask.
func newAncestrySource() launcher.AncestrySource {
	return launcher.NewToolhelpAncestry()
}

// newCgroupSource returns how a process's control-group membership is read on
// this OS, which is not at all: Windows has no control groups.
func newCgroupSource() launcher.CgroupSource {
	return launcher.NoCgroups{}
}
