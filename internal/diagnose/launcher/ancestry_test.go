package launcher

import (
	"context"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

// procParent is one entry in a fake process tree.
type procParent struct {
	ppid int
	name string
}

// fakeAncestry is a fixed pid → parent map standing in for /proc.
type fakeAncestry map[int]procParent

func (f fakeAncestry) Parent(_ context.Context, pid int) (int, string, bool) {
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
	want := []ProcInfo{{100, "ssh-agent"}, {50, "bash"}, {1, "systemd"}}
	assert.Equal(t, want, Ancestry(t.Context(), 100, tree), "the chain from the agent up to init")
}

func TestAncestryNilSource(t *testing.T) {
	assert.Nil(t, Ancestry(t.Context(), 100, nil), "with nothing to ask, there is no chain to report")
}

func TestAncestryMissingParent(t *testing.T) {
	// A parent absent from the tree stops the walk without error.
	got := Ancestry(t.Context(), 100, fakeAncestry{100: {ppid: 50, name: "ssh-agent"}})
	assert.Equal(t, []ProcInfo{{100, "ssh-agent"}}, got, "the walk stops where the tree does")
}

func TestAncestryCycle(t *testing.T) {
	tree := fakeAncestry{
		100: {ppid: 50, name: "a"},
		50:  {ppid: 100, name: "b"}, // points back → cycle
	}
	assert.Len(t, Ancestry(t.Context(), 100, tree), 2, "a tree that points back at itself must stop the walk, not loop it")
}

func TestAncestryDepthCap(t *testing.T) {
	tree := fakeAncestry{}
	for i := 1; i <= 100; i++ {
		tree[i] = procParent{ppid: i + 1, name: "p" + strconv.Itoa(i)}
	}
	assert.Len(t, Ancestry(t.Context(), 1, tree), maxAncestry, "a chain longer than the cap is cut at it")
}

func TestChainString(t *testing.T) {
	got := Chain([]ProcInfo{{100, "ssh-agent"}, {1, "systemd"}})
	assert.Equal(t, "ssh-agent(100) ← systemd(1)", got, "the chain as the report writes it")
}
