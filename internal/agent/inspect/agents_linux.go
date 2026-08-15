//go:build linux

package inspect

// platformAgents enumerates ssh-agent processes from the real Linux procfs
// tree at /proc.
func platformAgents() ([]AgentProc, error) {
	// Naming the kernel's own tree is this function's whole content; what is
	// made of what it holds is readProcfsTree's, and is checked against trees a
	// test writes.
	//coverage:ignore
	return readProcfsTree("/proc")
}
