package diagnose

import (
	"strconv"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	want := []ProcInfo{{100, "ssh-agent"}, {50, "bash"}, {1, "systemd"}}
	assert.Equal(t, want, ancestry(100, tree), "the chain from the agent up to init")
}

func TestAncestryNilSource(t *testing.T) {
	assert.Nil(t, ancestry(100, nil), "with nothing to ask, there is no chain to report")
}

func TestAncestryMissingParent(t *testing.T) {
	// A parent absent from the tree stops the walk without error.
	got := ancestry(100, fakeAncestry{100: {ppid: 50, name: "ssh-agent"}})
	assert.Equal(t, []ProcInfo{{100, "ssh-agent"}}, got, "the walk stops where the tree does")
}

func TestAncestryCycle(t *testing.T) {
	tree := fakeAncestry{
		100: {ppid: 50, name: "a"},
		50:  {ppid: 100, name: "b"}, // points back → cycle
	}
	assert.Len(t, ancestry(100, tree), 2, "a tree that points back at itself must stop the walk, not loop it")
}

func TestAncestryDepthCap(t *testing.T) {
	tree := fakeAncestry{}
	for i := 1; i <= 100; i++ {
		tree[i] = procParent{ppid: i + 1, name: "p" + strconv.Itoa(i)}
	}
	assert.Len(t, ancestry(1, tree), maxAncestry, "a chain longer than the cap is cut at it")
}

func TestChainString(t *testing.T) {
	got := chainString([]ProcInfo{{100, "ssh-agent"}, {1, "systemd"}})
	assert.Equal(t, "ssh-agent(100) ← systemd(1)", got, "the chain as the report writes it")
}

// fakeCgroup is a fixed pid → systemd unit map standing in for /proc/<pid>/cgroup.
type fakeCgroup map[int]string

func (f fakeCgroup) Cgroup(pid int) (string, bool) {
	unit, ok := f[pid]
	return unit, ok
}

// TestGatherForeignAttribution covers Gather naming whoever launched a foreign
// agent (F13). It is deliberately not platform-bound: the walk and the
// attribution are shared code, and `sshd` is a launcher both platforms' tables
// recognise under the same name — so this runs, and covers Gather, on each.
func TestGatherForeignAttribution(t *testing.T) {
	const foreign = "/tmp/foreign.sock"
	src := fakeSource{procs: []agent.AgentProc{
		{PID: 200, UID: 1000, Socket: foreign},
	}}
	prober := fakeProber{up: map[string]bool{foreign: true}}
	anc := fakeAncestry{
		200: {ppid: 8, name: "ssh-agent"},
		8:   {ppid: 1, name: "sshd"},
		1:   {ppid: 0, name: "init"},
	}
	r := Gather(Inputs{FixedSock: fixed, LegacyDir: legacy, EnvSock: fixed, OurUID: 1000}, src, prober, anc, nil, nil, nil)

	require.Len(t, r.Agents, 1, "the one agent that was found")
	assert.Len(t, r.Agents[0].Ancestry, 3, "the chain walked up from the agent")
	assert.Truef(t, hasFinding(r, "started by an SSH login session (sshd)"),
		"the report must say who started the foreign agent: %v", r.Findings)
}

// TestGatherRecordsWhatTheCgroupSourceReports covers Gather storing what a
// CgroupSource found, which is shared code and so has to be checked on every
// platform — including the one whose real source always reports nothing.
//
// What is done with the value afterwards is the platform's business (Linux can
// name a systemd unit from it, macOS has nothing comparable to name), and is
// asserted beside each platform's own labels.
func TestGatherRecordsWhatTheCgroupSourceReports(t *testing.T) {
	const foreign = "/tmp/foreign.sock"
	src := fakeSource{procs: []agent.AgentProc{{PID: 200, UID: 1000, Socket: foreign}}}
	prober := fakeProber{up: map[string]bool{foreign: true}}
	anc := fakeAncestry{200: {ppid: 1, name: "ssh-agent"}, 1: {ppid: 0, name: "init"}}
	cg := fakeCgroup{200: "app-gpg-agent.service"}

	r := Gather(Inputs{FixedSock: fixed, LegacyDir: legacy, EnvSock: fixed, OurUID: 1000}, src, prober, anc, cg, nil, nil)

	require.Len(t, r.Agents, 1, "the one agent that was found")
	assert.Equal(t, "app-gpg-agent.service", r.Agents[0].Cgroup, "the report carries what the cgroup source answered")
}
