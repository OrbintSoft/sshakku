package agent

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// fakeProc writes a /proc/<pid> entry with the given argv and (optional) status
// Uid line into root. A negative uid omits the status file, simulating a process
// whose owner we cannot read.
func fakeProc(t *testing.T, root string, pid int, argv []string, uid int) {
	t.Helper()
	dir := filepath.Join(root, strconv.Itoa(pid))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cmdline := strings.Join(argv, "\x00")
	if len(argv) > 0 {
		cmdline += "\x00" // the kernel NUL-terminates the final arg too
	}
	if err := os.WriteFile(filepath.Join(dir, "cmdline"), []byte(cmdline), 0o644); err != nil {
		t.Fatal(err)
	}
	if uid >= 0 {
		status := "Name:\tssh-agent\nUid:\t" + strconv.Itoa(uid) + "\t" + strconv.Itoa(uid) + "\t" + strconv.Itoa(uid) + "\t" + strconv.Itoa(uid) + "\n"
		if err := os.WriteFile(filepath.Join(dir, "status"), []byte(status), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

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
	fakeProc(t, root, 100, []string{"ssh-agent", "-a", "/run/user/1000/sshakku/tok/agent.sock"}, 1000)
	fakeProc(t, root, 200, []string{"/usr/bin/ssh-agent", "-a", "/home/u/.ssh/agent/ssh-agent.sock"}, 1000)
	fakeProc(t, root, 300, []string{"ssh-agent", "-D"}, 1001)                 // foreign, no -a, other user
	fakeProc(t, root, 400, []string{"ssh-agent", "-a/tmp/joined.sock"}, 1000) // joined -a form
	fakeProc(t, root, 500, []string{"/bin/bash", "-l"}, 1000)                 // not an agent
	fakeProc(t, root, 600, nil, 1000)                                         // kernel thread, empty cmdline
	fakeProc(t, root, 700, []string{"ssh-agent", "-a", "/tmp/noid.sock"}, -1) // owner unknown

	// A non-pid entry must be ignored.
	if err := os.MkdirAll(filepath.Join(root, "net"), 0o755); err != nil {
		t.Fatal(err)
	}

	in := Inspector{ProcRoot: root}
	procs, err := in.Agents()
	if err != nil {
		t.Fatalf("Agents: %v", err)
	}

	wantPIDs := map[int]bool{100: true, 200: true, 300: true, 400: true, 700: true}
	if len(procs) != len(wantPIDs) {
		t.Fatalf("got %d agents %v, want %d", len(procs), procs, len(wantPIDs))
	}
	for _, p := range procs {
		if !wantPIDs[p.PID] {
			t.Errorf("unexpected agent pid %d", p.PID)
		}
	}

	if p, _ := findPID(procs, 100); p.Socket != "/run/user/1000/sshakku/tok/agent.sock" || p.UID != 1000 {
		t.Errorf("pid 100: got socket=%q uid=%d", p.Socket, p.UID)
	}
	if p, _ := findPID(procs, 300); p.Socket != "" || p.UID != 1001 {
		t.Errorf("pid 300: got socket=%q uid=%d, want empty socket, uid 1001", p.Socket, p.UID)
	}
	if p, _ := findPID(procs, 400); p.Socket != "/tmp/joined.sock" {
		t.Errorf("pid 400: joined -a form, got socket=%q", p.Socket)
	}
	if p, _ := findPID(procs, 700); p.UID != -1 {
		t.Errorf("pid 700: missing status, got uid=%d, want -1", p.UID)
	}
}

func TestInspectorAgentsMissingRoot(t *testing.T) {
	in := Inspector{ProcRoot: filepath.Join(t.TempDir(), "nope")}
	if _, err := in.Agents(); err == nil {
		t.Fatal("want error for a missing procfs root")
	}
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
			got := Classify(AgentProc{Socket: c.socket}, fixed, legacyDir)
			if got != c.want {
				t.Errorf("Classify(%q) = %v, want %v", c.socket, got, c.want)
			}
		})
	}
}

func TestProcKindString(t *testing.T) {
	for k, want := range map[ProcKind]string{KindOurs: "ours", KindLegacy: "legacy", KindForeign: "foreign"} {
		if got := k.String(); got != want {
			t.Errorf("ProcKind(%d).String() = %q, want %q", k, got, want)
		}
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
			if got := socketArg(c.argv); got != c.want {
				t.Errorf("socketArg(%v) = %q, want %q", c.argv, got, c.want)
			}
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
		got := parseKernProcArgs2(buf)
		if len(got) != len(want) {
			t.Fatalf("parseKernProcArgs2 = %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("argv[%d] = %q, want %q", i, got[i], want[i])
			}
		}
	})
	t.Run("with word-alignment padding", func(t *testing.T) {
		want := []string{"ssh-agent", "-D"}
		buf := buildKernProcArgs2("/usr/bin/ssh-agent", 5, want)
		got := parseKernProcArgs2(buf)
		if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Fatalf("parseKernProcArgs2 = %v, want %v", got, want)
		}
	})
	t.Run("buffer too short", func(t *testing.T) {
		if got := parseKernProcArgs2([]byte{1, 2}); got != nil {
			t.Errorf("parseKernProcArgs2(short) = %v, want nil", got)
		}
	})
	t.Run("zero argc", func(t *testing.T) {
		buf := buildKernProcArgs2("/usr/bin/ssh-agent", 0, nil)
		if got := parseKernProcArgs2(buf); got != nil {
			t.Errorf("parseKernProcArgs2(argc=0) = %v, want nil", got)
		}
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
		if len(got) == 0 || got[0] != "ssh-agent" {
			t.Fatalf("parseKernProcArgs2(truncated) = %v, want a slice starting with ssh-agent", got)
		}
		if len(got) > 2 {
			t.Errorf("parseKernProcArgs2(truncated) returned %d entries from a 1-entry buffer, want at most 2 (real arg + trailing empty)", len(got))
		}
	})
}
