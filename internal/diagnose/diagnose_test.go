package diagnose

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OrbintSoft/sshakku/internal/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSource returns a fixed set of agent processes and an optional error.
type fakeSource struct {
	procs []agent.AgentProc
	err   error
}

func (f fakeSource) Agents() ([]agent.AgentProc, error) { return f.procs, f.err }

// fakeProber reports a socket reachable iff it is in the up set.
type fakeProber struct{ up map[string]bool }

func (f fakeProber) Reachable(_ context.Context, sock string) bool { return f.up[sock] }

const (
	fixed  = "/run/user/1000/sshakku/tok/agent.sock"
	legacy = "/home/u/.ssh/agent"
)

// hasFinding reports whether any finding contains sub.
func hasFinding(r Report, sub string) bool {
	for _, f := range r.Findings {
		if strings.Contains(f, sub) {
			return true
		}
	}
	return false
}

func TestGatherHealthy(t *testing.T) {
	src := fakeSource{procs: []agent.AgentProc{
		{PID: 100, UID: 1000, Socket: fixed, Args: []string{"ssh-agent", "-a", fixed}},
	}}
	prober := fakeProber{up: map[string]bool{fixed: true}}

	r := Gather(t.Context(), Inputs{
		FixedSock: fixed,
		LegacyDir: legacy,
		EnvSock:   fixed,
		OurUID:    1000,
		// A healthy shell has the broker wired: without it a key that expires
		// from the agent starts prompting again, which is a problem to report.
		EnvAskpass:        "/usr/bin/sshakku",
		EnvAskpassRequire: "force",
	}, src, prober, nil, nil, nil, nil)

	require.Len(t, r.Agents, 1, "the one agent that was running")
	a := r.Agents[0]
	assert.Equal(t, agent.KindOurs, a.Kind, "an agent on our fixed socket is ours")
	assert.True(t, a.Reachable, "the agent answers on its socket")
	assert.True(t, r.EnvReachable, "the socket the shell exported answers too")
	assert.Equal(t, StateOursHealthy, r.State, "one healthy agent of ours and nothing else")
	assert.Truef(t, hasFinding(r, "no problems detected"), "a healthy machine must be reported clean: %v", r.Findings)
}

func TestGatherEnvUnset(t *testing.T) {
	src := fakeSource{procs: []agent.AgentProc{
		{PID: 100, UID: 1000, Socket: fixed},
	}}
	prober := fakeProber{up: map[string]bool{fixed: true}}

	r := Gather(t.Context(), Inputs{FixedSock: fixed, LegacyDir: legacy, OurUID: 1000}, src, prober, nil, nil, nil, nil)
	assert.Truef(t, hasFinding(r, "SSH_AUTH_SOCK is unset"),
		"a shell that exported no socket must be told so: %v", r.Findings)
	// The clean bill is what a user reads first. It is added only when nothing
	// else was found, and a report carrying it alongside a problem would say
	// both things at once.
	assert.Falsef(t, hasFinding(r, "no problems detected"),
		"a report that found something must not also call the machine clean: %v", r.Findings)
}

func TestGatherEnvNotAnswering(t *testing.T) {
	src := fakeSource{}
	prober := fakeProber{} // nothing up
	r := Gather(t.Context(), Inputs{FixedSock: fixed, LegacyDir: legacy, EnvSock: fixed, OurUID: 1000}, src, prober, nil, nil, nil, nil)

	assert.False(t, r.EnvReachable, "nothing answers on the socket the shell exported")
	assert.Truef(t, hasFinding(r, "not answering"),
		"the report must say the exported socket is dead: %v", r.Findings)
	assert.Truef(t, hasFinding(r, "no ssh-agent is answering"),
		"the report must say there is no agent at all: %v", r.Findings)
}

func TestGatherEnvMismatch(t *testing.T) {
	const other = "/tmp/other.sock"
	src := fakeSource{procs: []agent.AgentProc{
		{PID: 100, UID: 1000, Socket: other},
	}}
	prober := fakeProber{up: map[string]bool{other: true}}
	r := Gather(t.Context(), Inputs{FixedSock: fixed, LegacyDir: legacy, EnvSock: other, OurUID: 1000}, src, prober, nil, nil, nil, nil)

	assert.Truef(t, hasFinding(r, "not our fixed socket"),
		"a shell pointed at somebody else's agent must be told so: %v", r.Findings)
}

func TestGatherMultipleAndDead(t *testing.T) {
	const foreign = "/tmp/foreign.sock"
	src := fakeSource{procs: []agent.AgentProc{
		{PID: 100, UID: 1000, Socket: fixed},                      // ours, reachable
		{PID: 200, UID: 1000, Socket: foreign},                    // foreign, reachable
		{PID: 300, UID: 1000, Socket: legacy + "/ssh-agent.sock"}, // legacy, dead
	}}
	prober := fakeProber{up: map[string]bool{fixed: true, foreign: true}}
	r := Gather(t.Context(), Inputs{FixedSock: fixed, LegacyDir: legacy, EnvSock: fixed, OurUID: 1000}, src, prober, nil, nil, nil, nil)

	assert.Truef(t, hasFinding(r, "2 agents are answering"),
		"the report must count the agents that answer: %v", r.Findings)
	assert.Truef(t, hasFinding(r, "1 dead ssh-agent"),
		"the report must count the ones that do not: %v", r.Findings)

	kinds := map[int]agent.ProcKind{}
	for _, a := range r.Agents {
		kinds[a.PID] = a.Kind
	}
	assert.Equal(t, agent.KindForeign, kinds[200], "an agent on a socket of its own is somebody else's")
	assert.Equal(t, agent.KindLegacy, kinds[300], "an agent under the legacy directory is a leftover of ours")
}

func TestGatherDifferentUserAgent(t *testing.T) {
	const other = "/run/user/1000/sshakku/tok/agent.sock"
	src := fakeSource{procs: []agent.AgentProc{
		{PID: 100, UID: 1000, Socket: other}, // healthy, but not uid 0's
	}}
	prober := fakeProber{up: map[string]bool{other: true}}
	r := Gather(t.Context(), Inputs{FixedSock: fixed, LegacyDir: legacy, OurUID: 0}, src, prober, nil, nil, nil, nil)

	assert.Equal(t, StateClean, r.State, "another user's agent is not serving this account")
	assert.Falsef(t, hasFinding(r, "foreign ssh-agent"),
		"another user's agent is not a foreign agent on this account: %v", r.Findings)
	assert.Falsef(t, hasFinding(r, "dead ssh-agent"),
		"another user's agent is not this account's leftover: %v", r.Findings)
	assert.Truef(t, hasFinding(r, "belong to a different user account"),
		"the report must say whose the agent is: %v", r.Findings)
}

// TestGatherAnotherUsersForeignAgentIsNotThisAccountsProblem pins the other
// half of the different-user rule: an agent on a socket that is nobody's
// business of ours is still not reported as serving this account when it
// belongs to somebody else. It is named as another user's and nothing more.
func TestGatherAnotherUsersForeignAgentIsNotThisAccountsProblem(t *testing.T) {
	const theirs = "/tmp/theirs.sock"
	src := fakeSource{procs: []agent.AgentProc{{PID: 200, UID: 1000, Socket: theirs}}}
	prober := fakeProber{up: map[string]bool{theirs: true}}
	r := Gather(t.Context(), Inputs{FixedSock: fixed, LegacyDir: legacy, OurUID: 0}, src, prober, nil, nil, nil, nil)

	assert.Falsef(t, hasFinding(r, "foreign ssh-agent"),
		"an agent belonging to another account is not a foreign agent serving this one: %v", r.Findings)
	assert.Truef(t, hasFinding(r, "belong to a different user account"),
		"the report must still account for it: %v", r.Findings)
}

func TestGatherOrphanedOursAgent(t *testing.T) {
	// Same shape sshakku itself uses, but a token that doesn't match this
	// session's own fixedSock — most likely a previous instance of our own
	// agent, not a truly external tool.
	const orphan = "/run/user/1000/sshakku/00112233445566778899aabbccddeeff/agent.sock"
	src := fakeSource{procs: []agent.AgentProc{
		{PID: 100, UID: 1000, Socket: orphan},
	}}
	prober := fakeProber{up: map[string]bool{orphan: true}}
	r := Gather(t.Context(), Inputs{FixedSock: fixed, LegacyDir: legacy, OurUID: 1000}, src, prober, nil, nil, nil, nil)

	assert.Truef(t, hasFinding(r, "looks like a previous sshakku-managed agent"),
		"an agent on a socket of our own shape is one of ours left behind: %v", r.Findings)
	assert.Falsef(t, hasFinding(r, "an unknown launcher"),
		"an agent we can account for must not be reported as unexplained: %v", r.Findings)
}

func TestLooksLikeOrphanedOurs(t *testing.T) {
	cases := []struct {
		socket string
		want   bool
	}{
		{"/run/user/1000/sshakku/00112233445566778899aabbccddeeff/agent.sock", true},
		{"/home/u/.cache/sshakku/00112233445566778899aabbccddeeff/agent.sock", true},
		{"/run/user/1000/sshakku/agent.sock", false},                                    // tokenless layout, no hex dir
		{"/run/user/1000/sshakku/TooShortHex/agent.sock", false},                        // wrong length
		{"/run/user/1000/sshakku/abc123/agent.sock", false},                             // lower hex, but not a token's worth
		{"/run/user/1000/sshakku/00112233445566778899aabbccddeeff/other.sock", false},   // right place, not our socket
		{"/run/user/1000/sshakku/00112233445566778899AABBCCDDEEFF/agent.sock", false},   // uppercase
		{"/run/user/1000/other-app/00112233445566778899aabbccddeeff/agent.sock", false}, // not sshakku
		{"/tmp/foreign.sock", false},
		{"", false},
	}
	for _, c := range cases {
		assert.Equalf(t, c.want, looksLikeOrphanedOurs(c.socket),
			"whether %q is the shape our own agent's socket has", c.socket)
	}
}

func TestKnownForeignShape(t *testing.T) {
	cases := []struct {
		socket string
		want   bool
	}{
		{"/run/user/1000/gnupg/S.gpg-agent.ssh", true},
		{"/home/u/.gnupg/S.gpg-agent.ssh", true},
		{"/run/user/1000/keyring/ssh", true},
		{"/run/user/1000/ssh-agent.socket", true},
		{"/run/user/1000/gnupg/S.gpg-agent", false}, // the main agent socket, not the ssh one
		{"/run/user/1000/keyring/pkcs11", false},    // a real gnome-keyring socket, wrong one
		{"/run/user/1000/elsewhere/ssh", false},     // the right name in the wrong place
		{"/tmp/keyring/ssh-agent.socket", true},     // basename alone identifies the systemd unit
		{"/tmp/foreign.sock", false},
		{"", false},
	}
	for _, c := range cases {
		_, ok := knownForeignShape(c.socket)
		assert.Equalf(t, c.want, ok, "whether %q is a socket shape the report can name", c.socket)
	}
}

func TestGatherEnvSockKnownForeignShape(t *testing.T) {
	const gpgSSH = "/run/user/1000/gnupg/S.gpg-agent.ssh"
	r := Gather(t.Context(), Inputs{FixedSock: fixed, LegacyDir: legacy, EnvSock: gpgSSH, OurUID: 1000},
		fakeSource{}, fakeProber{up: map[string]bool{gpgSSH: true}}, nil, nil, nil, nil)

	assert.Truef(t, hasFinding(r, "gpg-agent, with ssh support enabled"),
		"a socket shape the report knows must be named rather than called foreign: %v", r.Findings)
}

func TestGatherInspectError(t *testing.T) {
	src := fakeSource{err: errors.New("boom")}
	r := Gather(t.Context(), Inputs{FixedSock: fixed, LegacyDir: legacy, EnvSock: fixed, OurUID: 1000},
		src, fakeProber{up: map[string]bool{fixed: true}}, nil, nil, nil, nil)
	assert.Error(t, r.InspectErr, "processes that could not be listed must be reported")
	assert.Truef(t, hasFinding(r, "could not enumerate processes"),
		"the user must be told the scan did not happen: %v", r.Findings)
}

func TestGatherRecordedPID(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "agent.state")
	require.NoError(t, agent.WriteState(statePath, agent.State{PID: 4242, Socket: fixed}), "seed the state file")
	r := Gather(t.Context(), Inputs{FixedSock: fixed, LegacyDir: legacy, EnvSock: fixed, StatePath: statePath, OurUID: 1000},
		fakeSource{}, fakeProber{up: map[string]bool{fixed: true}}, nil, nil, nil, nil)
	assert.Equal(t, 4242, r.RecordedPID, "the pid the state file recorded")
}

func TestGatherAskpassNotWired(t *testing.T) {
	r := Gather(t.Context(), Inputs{FixedSock: fixed, LegacyDir: legacy, EnvSock: fixed, OurUID: 1000},
		fakeSource{}, fakeProber{up: map[string]bool{fixed: true}}, nil, nil, nil, nil)
	assert.Truef(t, hasFinding(r, "SSH_ASKPASS is not wired"),
		"a shell with no askpass wired must be told so: %v", r.Findings)
}

func TestGatherAskpassPartiallyWired(t *testing.T) {
	r := Gather(t.Context(), Inputs{
		FixedSock: fixed, LegacyDir: legacy, EnvSock: fixed, OurUID: 1000,
		EnvAskpass: "/usr/bin/sshakku",
	}, fakeSource{}, fakeProber{up: map[string]bool{fixed: true}}, nil, nil, nil, nil)
	assert.Truef(t, hasFinding(r, "SSH_ASKPASS is not wired"),
		"an askpass OpenSSH may ignore is not wired: %v", r.Findings)
}

func TestGatherAskpassWired(t *testing.T) {
	src := fakeSource{procs: []agent.AgentProc{
		{PID: 100, UID: 1000, Socket: fixed, Args: []string{"ssh-agent", "-a", fixed}},
	}}
	r := Gather(t.Context(), Inputs{
		FixedSock: fixed, LegacyDir: legacy, EnvSock: fixed, OurUID: 1000,
		EnvAskpass: "/usr/bin/sshakku", EnvAskpassRequire: "force",
	}, src, fakeProber{up: map[string]bool{fixed: true}}, nil, nil, nil, nil)
	assert.Falsef(t, hasFinding(r, "SSH_ASKPASS is not wired"),
		"a shell with the askpass wired must not be told otherwise: %v", r.Findings)
	assert.Truef(t, hasFinding(r, "no problems detected"),
		"a healthy machine must be reported clean: %v", r.Findings)
}

// TestGatherAskpassNotWiredHeadless covers the session that needs the finding
// most: one with no display server at all, where nothing else will explain why
// a passphrase is being asked for on the terminal.
func TestGatherAskpassNotWiredHeadless(t *testing.T) {
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	r := Gather(t.Context(), Inputs{FixedSock: fixed, LegacyDir: legacy, EnvSock: fixed, OurUID: 1000},
		fakeSource{}, fakeProber{up: map[string]bool{fixed: true}}, nil, nil, nil, nil)
	assert.Truef(t, hasFinding(r, "SSH_ASKPASS is not wired"),
		"a session with no display is the one that most needs to be told: %v", r.Findings)
}

func TestTailLines(t *testing.T) {
	dir := t.TempDir()

	assert.Nil(t, tailLines(filepath.Join(dir, "missing.log"), 5), "a log that is not there has no tail")

	empty := filepath.Join(dir, "empty.log")
	require.NoError(t, os.WriteFile(empty, []byte("\n\n"), 0o600), "write the empty log")
	assert.Nil(t, tailLines(empty, 5), "a log of blank lines has no tail either")

	full := filepath.Join(dir, "full.log")
	require.NoError(t, os.WriteFile(full, []byte("l1\nl2\nl3\nl4\n"), 0o600), "write the log")
	assert.Equal(t, []string{"l3", "l4"}, tailLines(full, 2), "the last two lines, in the order they were written")
	assert.Len(t, tailLines(full, 10), 4, "asking for more lines than there are yields all of them")
}

func TestHostFindings(t *testing.T) {
	no, yes := false, true

	// Eight independent questions about one report; assert throughout, and
	// only look inside a finding once there is exactly one to look inside.
	assert.Empty(t, hostFindings(HostChecks{}), "a host nothing was established about yields nothing to report")

	got := hostFindings(HostChecks{DiskEncrypted: &no})
	if assert.Len(t, got, 1, "an unencrypted disk is one finding") {
		assert.Contains(t, got[0], "not appear to be encrypted", "the finding must say the disk is not encrypted")
	}

	assert.Empty(t, hostFindings(HostChecks{DiskEncrypted: &yes}), "an encrypted disk is nothing to report")

	got = hostFindings(HostChecks{TmpTmpfs: &no})
	if assert.Len(t, got, 1, "a /tmp that is not a tmpfs is one finding") {
		assert.Contains(t, got[0], "not a dedicated tmpfs mount", "the finding must say what /tmp is not")
	}

	got = hostFindings(HostChecks{TmpTmpfs: &yes, TmpSizeBytes: 64 * 1024 * 1024})
	if assert.Len(t, got, 1, "a tmpfs too small to be useful is one finding") {
		assert.Contains(t, got[0], "too small", "the finding must say the tmpfs is undersized")
	}

	assert.Empty(t, hostFindings(HostChecks{TmpTmpfs: &yes, TmpSizeBytes: 2 * 1024 * 1024 * 1024}),
		"a tmpfs of adequate size is nothing to report")

	got = hostFindings(HostChecks{SecureHardwarePresent: &no})
	if assert.Len(t, got, 1, "a machine with no secure hardware is one finding") {
		assert.Contains(t, got[0], "no TPM or Secure Enclave detected", "the finding must say none was found")
	}

	assert.Empty(t, hostFindings(HostChecks{SecureHardwarePresent: &yes, SecureHardwareKind: "TPM 2.0"}),
		"secure hardware that is present is nothing to report")
}

func TestHostChecksLine(t *testing.T) {
	no, yes := false, true

	assert.Empty(t, hostChecksLine(HostChecks{}), "a host that was never looked at has no line to print")

	got := hostChecksLine(HostChecks{DiskEncrypted: &yes, TmpTmpfs: &yes, TmpSizeBytes: 1024 * 1024 * 1024, SecureHardwarePresent: &yes, SecureHardwareKind: "TPM 2.0"})
	for _, want := range []string{"disk encryption: yes", "/tmp: tmpfs, 1.0 GiB", "secure hardware: present (TPM 2.0)"} {
		assert.Containsf(t, got, want, "the line must say %q", want)
	}

	got = hostChecksLine(HostChecks{DiskEncrypted: &no, TmpTmpfs: &no, SecureHardwarePresent: &no})
	for _, want := range []string{"disk encryption: no", "/tmp: not tmpfs", "secure hardware: not detected"} {
		assert.Containsf(t, got, want, "the line must say %q", want)
	}
}

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{
		512:                "512 B",
		2048:               "2.0 KiB",
		512 * 1024 * 1024:  "512.0 MiB",
		1024 * 1024 * 1024: "1.0 GiB",
	}
	for n, want := range cases {
		assert.Equalf(t, want, humanBytes(n), "how %d bytes is written for a person", n)
	}
}

func TestFormatIncludesEnvironmentSection(t *testing.T) {
	yes := true
	r := Report{
		FixedSock: fixed,
		Findings:  []string{"no problems detected"},
		Host:      HostChecks{DiskEncrypted: &yes, TmpTmpfs: &yes, TmpSizeBytes: 1024 * 1024 * 1024, SecureHardwarePresent: &yes, SecureHardwareKind: "TPM 2.0"},
	}
	var buf bytes.Buffer
	Format(&buf, r)
	out := buf.String()
	assert.Contains(t, out, "environment:", "the report must have a section for the host")
	assert.Contains(t, out, "disk encryption: yes", "the section must carry what was established")
}

func TestFormatOmitsEnvironmentSectionWhenUngathered(t *testing.T) {
	r := Report{FixedSock: fixed, Findings: []string{"no problems detected"}}
	var buf bytes.Buffer
	Format(&buf, r)
	assert.NotContains(t, buf.String(), "environment:",
		"the report must leave out a section nothing was gathered for")
}

func TestFormat(t *testing.T) {
	r := Report{
		FixedSock:    fixed,
		EnvSock:      fixed,
		EnvReachable: true,
		OurUID:       1000,
		RecordedPID:  4242,
		State:        StateOursHealthy,
		Agents: []AgentView{
			{PID: 100, UID: 1000, Kind: agent.KindOurs, Socket: fixed, Reachable: true},
			{PID: 200, UID: 1001, Kind: agent.KindForeign, Socket: "/tmp/f.sock", Reachable: false},
			{PID: 300, UID: -1, Kind: agent.KindForeign, Socket: "", Reachable: false},
		},
		Findings: []string{"no problems detected"},
		LogTail:  []string{"2026-07-01T00:00:00Z INFO started"},
	}
	var buf bytes.Buffer
	Format(&buf, r)
	out := buf.String()

	for _, want := range []string{
		"ssh-agent diagnostics",
		"state: B —",
		"fixed socket:  " + fixed,
		"(reachable)",
		"recorded pid:  4242",
		"pid 100",
		"you",      // our own agent
		"uid 1001", // another user's agent
		"uid ?",    // unknown owner
		"reachable",
		"dead",
		"no problems detected",
		"recommendation:",
		"recent log:",
		"INFO started",
	} {
		assert.Containsf(t, out, want, "the report must say %q", want)
	}
}
