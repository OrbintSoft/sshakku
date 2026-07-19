//go:build darwin

package agent

import (
	"fmt"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// platformAgents enumerates ssh-agent processes via sysctl: macOS has no
// /proc for readProcfsTree to read, so this uses kern.proc.all for the
// process list and kern.procargs2 per pid for argv — the same sysctls ps(1)
// itself uses.
func platformAgents() ([]AgentProc, error) {
	kprocs, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil, fmt.Errorf("sysctl kern.proc.all: %w", err)
	}
	var procs []AgentProc
	for _, kp := range kprocs {
		pid := int(kp.Proc.P_pid)
		if pid <= 0 {
			continue
		}
		argv, err := darwinProcArgs(pid)
		if err != nil || len(argv) == 0 {
			continue // process gone, or its args aren't visible to us (another user's)
		}
		if filepath.Base(argv[0]) != "ssh-agent" {
			continue
		}
		procs = append(procs, AgentProc{
			PID:    pid,
			UID:    int(kp.Eproc.Pcred.P_ruid),
			Socket: socketArg(argv),
			Args:   argv,
		})
	}
	return procs, nil
}

// darwinProcArgs reads pid's argv via the kern.procargs2 sysctl, the macOS
// equivalent of Linux's /proc/<pid>/cmdline. See parseKernProcArgs2 for the
// buffer layout.
func darwinProcArgs(pid int) ([]string, error) {
	buf, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil {
		return nil, err
	}
	return parseKernProcArgs2(buf), nil
}
