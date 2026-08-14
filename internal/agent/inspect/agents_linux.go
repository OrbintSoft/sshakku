//go:build linux

package inspect

// platformAgents enumerates ssh-agent processes from the real Linux procfs
// tree at /proc.
func platformAgents() ([]AgentProc, error) {
	return readProcfsTree("/proc")
}
