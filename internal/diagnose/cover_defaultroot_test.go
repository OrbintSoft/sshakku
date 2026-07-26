package diagnose

import "testing"

// These cover the empty-Root default to /proc in ProcfsAncestry.Parent and
// ProcfsCgroup.Cgroup. They are portable — a pid that cannot exist makes the
// read fail deterministically once the default root is applied, whether or not
// /proc exists on the host — so they run off Linux too, where these types are
// still compiled (and degrade to not-ok) even though nothing reads /proc there.

// TestProcfsAncestryDefaultRoot covers Parent's empty-Root default to /proc.
func TestProcfsAncestryDefaultRoot(t *testing.T) {
	if _, _, ok := (ProcfsAncestry{}).Parent(1 << 30); ok {
		t.Error("Parent of a nonexistent pid under /proc must report not-ok")
	}
}

// TestProcfsCgroupDefaultRoot covers Cgroup's empty-Root default to /proc.
func TestProcfsCgroupDefaultRoot(t *testing.T) {
	if _, ok := (ProcfsCgroup{}).Cgroup(1 << 30); ok {
		t.Error("Cgroup of a nonexistent pid under /proc must report not-ok")
	}
}
