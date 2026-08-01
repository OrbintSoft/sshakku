package diagnose

import (
	"github.com/OrbintSoft/sshakku/internal/agent"
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

	if len(r.Agents) != 1 || len(r.Agents[0].Ancestry) != 3 {
		t.Fatalf("ancestry not populated: %+v", r.Agents)
	}
	if !hasFinding(r, "started by an SSH login session (sshd)") {
		t.Errorf("findings = %v, want a foreign-attribution finding", r.Findings)
	}
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

	if len(r.Agents) != 1 {
		t.Fatalf("agents = %+v, want exactly one", r.Agents)
	}
	if r.Agents[0].Cgroup != "app-gpg-agent.service" {
		t.Errorf("Cgroup = %q, want what the source reported", r.Agents[0].Cgroup)
	}
}
