package launcher

// CgroupSource reports what a process's control-group membership can still
// say about who launched it, when the process tree no longer can. Each
// platform supplies its own; tests supply a fake.
type CgroupSource interface {
	Cgroup(pid int) (unit string, ok bool)
}
