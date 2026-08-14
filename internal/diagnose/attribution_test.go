package diagnose

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/agent"
	"github.com/OrbintSoft/sshakku/internal/diagnose/launcher"
)

// procParent, fakeAncestry and fakeCgroup stand in for what a report reads
// about the process tree and the control groups. They are the two seams Gather
// is given, so a test here decides what the machine looked like instead of
// asking the one it runs on.
type procParent struct {
	ppid int
	name string
}

// fakeAncestry is a fixed pid to parent map standing in for the process tree.
type fakeAncestry map[int]procParent

func (f fakeAncestry) Parent(_ context.Context, pid int) (int, string, bool) {
	e, ok := f[pid]
	if !ok {
		return 0, "", false
	}
	return e.ppid, e.name, true
}

// fakeCgroup is a fixed pid to unit map standing in for a process's cgroup.
type fakeCgroup map[int]string

func (f fakeCgroup) Cgroup(pid int) (string, bool) {
	unit, ok := f[pid]
	return unit, ok
}

var (
	_ launcher.AncestrySource = fakeAncestry{}
	_ launcher.CgroupSource   = fakeCgroup{}
)

// TestGatherForeignAttribution covers Gather naming whoever launched a foreign
// agent (F13). It is deliberately not platform-bound: the walk and the
// attribution are shared code, so this runs, and covers Gather, on every
// system. What each of them calls that launcher is theirs to say — a platform
// whose table recognises sshd gives it a name, one whose table recognises
// nothing falls back to the parent's own — so the expected description is
// wantSSHLauncher rather than one platform's wording.
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
	r := Gather(t.Context(), Inputs{FixedSock: fixed, LegacyDir: legacy, EnvSock: fixed, OurUID: 1000}, src, prober, anc, nil, nil, nil)

	require.Len(t, r.Agents, 1, "the one agent that was found")
	assert.Len(t, r.Agents[0].Ancestry, 3, "the chain walked up from the agent")
	assert.Truef(t, hasFinding(r, "started by "+wantSSHLauncher),
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

	r := Gather(t.Context(), Inputs{FixedSock: fixed, LegacyDir: legacy, EnvSock: fixed, OurUID: 1000}, src, prober, anc, cg, nil, nil)

	require.Len(t, r.Agents, 1, "the one agent that was found")
	assert.Equal(t, "app-gpg-agent.service", r.Agents[0].Cgroup, "the report carries what the cgroup source answered")
}
