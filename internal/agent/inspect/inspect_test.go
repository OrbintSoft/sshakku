package inspect

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OrbintSoft/sshakku/internal/agent/inspect/inspecttest"
)

func findPID(procs []AgentProc, pid int) (AgentProc, bool) {
	for _, p := range procs {
		if p.PID == pid {
			return p, true
		}
	}
	return AgentProc{}, false
}

func TestInspectorAgents(t *testing.T) {
	root := t.TempDir()
	inspecttest.FakeProc(t, root, 100, []string{"ssh-agent", "-a", "/run/user/1000/sshakku/tok/agent.sock"}, 1000)
	inspecttest.FakeProc(t, root, 200, []string{"/usr/bin/ssh-agent", "-a", "/home/u/.ssh/agent/ssh-agent.sock"}, 1000)
	inspecttest.FakeProc(t, root, 300, []string{"ssh-agent", "-D"}, 1001)                 // foreign, no -a, other user
	inspecttest.FakeProc(t, root, 400, []string{"ssh-agent", "-a/tmp/joined.sock"}, 1000) // joined -a form
	inspecttest.FakeProc(t, root, 500, []string{"/bin/bash", "-l"}, 1000)                 // not an agent
	inspecttest.FakeProc(t, root, 600, nil, 1000)                                         // kernel thread, empty cmdline
	inspecttest.FakeProc(t, root, 700, []string{"ssh-agent", "-a", "/tmp/noid.sock"}, -1) // owner unknown

	// A real /proc holds more than pid directories, in both shapes: other
	// directories (net, self, irq) and plain files (uptime, meminfo). Neither
	// is a process and neither may be reported as one.
	require.NoError(t, os.MkdirAll(filepath.Join(root, "net"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "uptime"), []byte("1234.56 789.01\n"), 0o644))

	in := Inspector{ProcRoot: root}
	procs, err := in.Agents()
	require.NoError(t, err, "Agents")

	gotPIDs := make([]int, 0, len(procs))
	for _, p := range procs {
		gotPIDs = append(gotPIDs, p.PID)
	}
	assert.ElementsMatch(t, []int{100, 200, 300, 400, 700}, gotPIDs, "the agents found, and only those")

	p100, _ := findPID(procs, 100)
	assert.Equal(t, "/run/user/1000/sshakku/tok/agent.sock", p100.Socket, "pid 100 socket")
	assert.Equal(t, 1000, p100.UID, "pid 100 uid")

	p300, _ := findPID(procs, 300)
	assert.Empty(t, p300.Socket, "pid 300 names no socket")
	assert.Equal(t, 1001, p300.UID, "pid 300 uid")

	p400, _ := findPID(procs, 400)
	assert.Equal(t, "/tmp/joined.sock", p400.Socket, "pid 400 uses the joined -a form")

	p700, _ := findPID(procs, 700)
	assert.Equal(t, -1, p700.UID, "pid 700 has no status file, so its owner is unknown")
}

func TestInspectorAgentsMissingRoot(t *testing.T) {
	in := Inspector{ProcRoot: filepath.Join(t.TempDir(), "nope")}
	_, err := in.Agents()
	assert.Error(t, err, "a missing procfs root must be reported")
}

func TestClassify(t *testing.T) {
	const fixed = "/run/user/1000/sshakku/tok/agent.sock"
	const legacyDir = "/home/u/.ssh/agent"

	cases := []struct {
		name   string
		socket string
		want   ProcKind
	}{
		{"ours", fixed, KindOurs},
		{"legacy", legacyDir + "/ssh-agent.sock", KindLegacy},
		{"foreign elsewhere", "/tmp/other.sock", KindForeign},
		{"no socket", "", KindForeign},
		{"legacy sibling not under", "/home/u/.ssh/agentX.sock", KindForeign},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equalf(t, c.want, Classify(AgentProc{Socket: c.socket}, fixed, legacyDir), "Classify(%q)", c.socket)
		})
	}
}

func TestProcKindString(t *testing.T) {
	for k, want := range map[ProcKind]string{KindOurs: "ours", KindLegacy: "legacy", KindForeign: "foreign"} {
		assert.Equalf(t, want, k.String(), "ProcKind(%d).String()", k)
	}
}

func TestSocketArg(t *testing.T) {
	cases := []struct {
		name string
		argv []string
		want string
	}{
		{"separated", []string{"ssh-agent", "-a", "/x.sock"}, "/x.sock"},
		{"joined", []string{"ssh-agent", "-a/x.sock"}, "/x.sock"},
		{"dangling -a", []string{"ssh-agent", "-a"}, ""},
		{"absent", []string{"ssh-agent", "-D", "-d"}, ""},
		{"after other flags", []string{"ssh-agent", "-D", "-a", "/x.sock"}, "/x.sock"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equalf(t, c.want, socketArg(c.argv), "socketArg(%v)", c.argv)
		})
	}
}

// buildKernProcArgs2 synthesizes a buffer shaped like what macOS's
// kern.procargs2 sysctl returns, for parseKernProcArgs2's unit tests:
// [4-byte argc][execPath\0][padding NULs][argv[0]\0]...[argv[n-1]\0].
func buildKernProcArgs2(execPath string, padding int, argv []string) []byte {
	var buf []byte
	argc := make([]byte, 4)
	binary.LittleEndian.PutUint32(argc, uint32(len(argv)))
	buf = append(buf, argc...)
	buf = append(buf, execPath...)
	buf = append(buf, 0)
	buf = append(buf, make([]byte, padding)...)
	for _, a := range argv {
		buf = append(buf, a...)
		buf = append(buf, 0)
	}
	return buf
}

func TestParseKernProcArgs2(t *testing.T) {
	t.Run("normal, no padding", func(t *testing.T) {
		want := []string{"ssh-agent", "-a", "/tmp/x.sock"}
		buf := buildKernProcArgs2("/usr/bin/ssh-agent", 0, want)
		assert.Equal(t, want, parseKernProcArgs2(buf), "parseKernProcArgs2")
	})
	t.Run("with word-alignment padding", func(t *testing.T) {
		want := []string{"ssh-agent", "-D"}
		buf := buildKernProcArgs2("/usr/bin/ssh-agent", 5, want)
		assert.Equal(t, want, parseKernProcArgs2(buf), "parseKernProcArgs2")
	})
	t.Run("buffer too short", func(t *testing.T) {
		assert.Nil(t, parseKernProcArgs2([]byte{1, 2}), "a buffer too short to hold argc yields nothing")
	})
	t.Run("zero argc", func(t *testing.T) {
		buf := buildKernProcArgs2("/usr/bin/ssh-agent", 0, nil)
		assert.Nil(t, parseKernProcArgs2(buf), "argc = 0 yields nothing")
	})
	t.Run("argc larger than available chunks does not panic or overrun", func(t *testing.T) {
		// A real kernel buffer's argc always matches its actual argv count;
		// this only exercises defensive bounds-checking against a
		// corrupted/truncated buffer, matching the same "prefer a trailing
		// empty string over reading into the environment" tradeoff
		// well-established parsers of this exact sysctl format make.
		buf := buildKernProcArgs2("/usr/bin/ssh-agent", 0, []string{"ssh-agent"})
		binary.LittleEndian.PutUint32(buf[:4], 5) // claim 5 args, only 1 present
		got := parseKernProcArgs2(buf)
		require.NotEmpty(t, got, "a truncated buffer must still yield the argument it does hold")
		assert.Equal(t, "ssh-agent", got[0], "the first argument")
		assert.LessOrEqual(t, len(got), 2,
			"a 1-entry buffer must yield at most 2 entries (the real arg plus a trailing empty one)")
	})
}
