package diagnose

import (
	"os"
	"path/filepath"
	"testing"
)

// These cover ProcfsAncestry.Parent and ProcfsCgroup.Cgroup branches that are
// reachable on any OS — these types are compiled off Linux too, where they
// degrade to not-ok even though nothing reads /proc there — so the tests are
// deliberately portable (no /proc, no Linux-only test helpers) to keep the
// branches covered on the macOS build as well as Linux.

// TestProcfsAncestryDefaultRoot covers Parent's empty-Root default to /proc.
func TestProcfsAncestryDefaultRoot(t *testing.T) {
	if _, _, ok := (ProcfsAncestry{}).Parent(1 << 30); ok {
		t.Error("Parent of a nonexistent pid under /proc must report not-ok")
	}
}

// TestProcfsAncestryReadError covers Parent's read-failure branch
// deterministically: a Root that does not exist makes the stat read fail
// regardless of whether the platform has a /proc at all.
func TestProcfsAncestryReadError(t *testing.T) {
	root := filepath.Join(t.TempDir(), "does-not-exist")
	if _, _, ok := (ProcfsAncestry{Root: root}).Parent(1); ok {
		t.Error("Parent with an unreadable stat must report not-ok")
	}
}

// TestProcfsAncestryParseFailure covers Parent's branch where the stat file is
// readable but unparseable.
func TestProcfsAncestryParseFailure(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "5")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stat"), []byte("no parentheses here"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := (ProcfsAncestry{Root: root}).Parent(5); ok {
		t.Error("Parent of a malformed stat must report not-ok")
	}
}

// TestProcfsCgroupDefaultRoot covers Cgroup's empty-Root default to /proc.
func TestProcfsCgroupDefaultRoot(t *testing.T) {
	if _, ok := (ProcfsCgroup{}).Cgroup(1 << 30); ok {
		t.Error("Cgroup of a nonexistent pid under /proc must report not-ok")
	}
}
