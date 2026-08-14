//go:build darwin

package launcher

// NoCgroups is the CgroupSource for a system that has no control groups.
//
// This is an absence, not a gap: macOS keeps no per-process record that
// survives a double-fork and still names what launched the process, so there is
// nothing here for a real implementation to read. Reporting nothing is the
// whole truth, and `startedBy` already says "an unknown launcher" when the
// process tree dead-ends.
type NoCgroups struct{}

// Cgroup reports nothing, always.
func (NoCgroups) Cgroup(int) (string, bool) { return "", false }

var _ CgroupSource = NoCgroups{}
