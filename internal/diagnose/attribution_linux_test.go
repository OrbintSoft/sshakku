//go:build linux

package diagnose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/OrbintSoft/sshakku/internal/agent"
)

func TestGatherReparentedToInitCgroupFallback(t *testing.T) {
	const foreign = "/tmp/foreign.sock"
	src := fakeSource{procs: []agent.AgentProc{
		{PID: 200, UID: 1000, Socket: foreign},
	}}
	prober := fakeProber{up: map[string]bool{foreign: true}}
	anc := fakeAncestry{
		200: {ppid: 1, name: "ssh-agent"},
		1:   {ppid: 0, name: "systemd"},
	}
	cg := fakeCgroup{200: "app-gpg-agent.service"}
	r := Gather(t.Context(), Inputs{FixedSock: fixed, LegacyDir: legacy, EnvSock: fixed, OurUID: 1000}, src, prober, anc, cg, nil, nil)

	assert.Truef(t, hasFinding(r, "systemd unit: app-gpg-agent.service"),
		"an agent whose parent is gone must still be named by its unit: %v", r.Findings)
}
