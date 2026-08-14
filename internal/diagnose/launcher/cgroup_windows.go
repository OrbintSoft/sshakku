//go:build windows

package launcher

// NoCgroups is the CgroupSource for a system that has no control groups.
//
// Windows groups processes in job objects, which is a different thing: a job
// is something a launcher opts into, not a record every process carries, so
// there is nothing here that reliably survives a process's parent and still
// names what started it. Reporting nothing is the whole truth; `startedBy`
// already says "an unknown launcher" when the process tree dead-ends.
type NoCgroups struct{}

// Cgroup reports nothing, always.
func (NoCgroups) Cgroup(int) (string, bool) { return "", false }

var _ CgroupSource = NoCgroups{}
