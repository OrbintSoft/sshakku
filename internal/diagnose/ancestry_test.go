package diagnose

import (
	"strconv"
	"testing"
)

// procParent is one entry in a fake process tree.
type procParent struct {
	ppid int
	name string
}

// fakeAncestry is a fixed pid → parent map standing in for /proc.
type fakeAncestry map[int]procParent

func (f fakeAncestry) Parent(pid int) (int, string, bool) {
	e, ok := f[pid]
	if !ok {
		return 0, "", false
	}
	return e.ppid, e.name, true
}

func TestAncestry(t *testing.T) {
	tree := fakeAncestry{
		100: {ppid: 50, name: "ssh-agent"},
		50:  {ppid: 1, name: "bash"},
		1:   {ppid: 0, name: "systemd"},
	}
	got := ancestry(100, tree)
	want := []ProcInfo{{100, "ssh-agent"}, {50, "bash"}, {1, "systemd"}}
	if len(got) != len(want) {
		t.Fatalf("ancestry = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("chain[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestAncestryNilSource(t *testing.T) {
	if got := ancestry(100, nil); got != nil {
		t.Errorf("ancestry(nil source) = %v, want nil", got)
	}
}

func TestAncestryMissingParent(t *testing.T) {
	// A parent absent from the tree stops the walk without error.
	got := ancestry(100, fakeAncestry{100: {ppid: 50, name: "ssh-agent"}})
	if len(got) != 1 || got[0].Name != "ssh-agent" {
		t.Errorf("chain = %v, want just the agent", got)
	}
}

func TestAncestryCycle(t *testing.T) {
	tree := fakeAncestry{
		100: {ppid: 50, name: "a"},
		50:  {ppid: 100, name: "b"}, // points back → cycle
	}
	if got := ancestry(100, tree); len(got) != 2 {
		t.Errorf("cycle: chain = %v, want 2 entries then stop", got)
	}
}

func TestAncestryDepthCap(t *testing.T) {
	tree := fakeAncestry{}
	for i := 1; i <= 100; i++ {
		tree[i] = procParent{ppid: i + 1, name: "p" + strconv.Itoa(i)}
	}
	if got := ancestry(1, tree); len(got) != maxAncestry {
		t.Errorf("depth cap: chain len = %d, want %d", len(got), maxAncestry)
	}
}

func TestChainString(t *testing.T) {
	got := chainString([]ProcInfo{{100, "ssh-agent"}, {1, "systemd"}})
	want := "ssh-agent(100) ← systemd(1)"
	if got != want {
		t.Errorf("chainString = %q, want %q", got, want)
	}
}

// fakeCgroup is a fixed pid → systemd unit map standing in for /proc/<pid>/cgroup.
type fakeCgroup map[int]string

func (f fakeCgroup) Cgroup(pid int) (string, bool) {
	unit, ok := f[pid]
	return unit, ok
}
