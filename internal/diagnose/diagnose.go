// Package diagnose builds a read-only picture of the ssh-agent situation for the
// `sshakku doctor` command: which ssh-agent processes are running, which one (if
// any) is ours, whether each answers, and whether the shell's SSH_AUTH_SOCK is
// wired to a healthy agent. It only reads state — it never starts, signals, or
// reaps anything.
package diagnose

import (
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/OrbintSoft/sshakku/internal/agent"
	"github.com/OrbintSoft/sshakku/internal/keystate"
)

//go:embed askpass_not_wired.txt
var askpassNotWiredMsgFile string

// askpassNotWiredMsg is the finding text for the askpass-wiring check, kept as
// its own file so the prose can be read and edited as plain text rather than a
// Go string literal.
var askpassNotWiredMsg = strings.TrimSpace(askpassNotWiredMsgFile)

// logTailLines is how many trailing session-log lines the report shows.
const logTailLines = 10

// now is the clock Format uses to render a loaded key's remaining time; a
// var, not a hard dependency, so tests can pin it.
var now = time.Now

// AgentSource enumerates the ssh-agent processes currently visible.
// agent.Inspector satisfies it; tests supply a fake.
type AgentSource interface {
	Agents() ([]agent.AgentProc, error)
}

// KeyLister lists the private-key files to consider; keys.Enumerator
// satisfies it.
type KeyLister interface {
	Keys() ([]string, error)
}

// KeyFingerprinter resolves a key file's fingerprint and the set currently
// loaded in the agent; keys.RunnerFingerprinter satisfies it.
type KeyFingerprinter interface {
	FileFingerprint(path string) (string, error)
	AgentFingerprints() (map[string]bool, error)
}

// KeyStateSource looks up the lifetime sshakku recorded for a key it added;
// keystate.Store satisfies it.
type KeyStateSource interface {
	Load(key string) (keystate.Record, bool)
}

// KeySource bundles the collaborators needed to inspect ~/.ssh keys and their
// agent/TTL state. A nil KeySource (the Gather parameter) skips the keys
// section entirely; a nil Lister field does the same.
type KeySource struct {
	Lister      KeyLister
	Fingerprint KeyFingerprinter
	State       KeyStateSource
}

// Inputs are the facts Gather reasons over, injected so it stays pure and
// testable — nothing here is read from the ambient process.
type Inputs struct {
	FixedSock string // the socket our agent binds (from the resolved layout)
	LegacyDir string // ~/.ssh/agent, for spotting a pre-sshakku agent
	StatePath string // agent.state, holding the pid of the agent we started
	EnvSock   string // SSH_AUTH_SOCK as this shell sees it
	LogFile   string // session log to tail
	OurUID    int    // the invoking user's uid, to tell same-user agents apart

	// GUIAvailable, EnvAskpass, and EnvAskpassRequire describe whether this
	// shell's ssh passphrase prompts are routed through sshakku's wallet-aware
	// askpass broker, mirroring the same condition `sshakku askpass-env` uses
	// to decide whether to wire it in. GUIAvailable is computed by the caller
	// (see keys.GUIAvailable) rather than here, keeping this package free of
	// any dependency on a display server or an external prompter binary.
	GUIAvailable      bool
	EnvAskpass        string // SSH_ASKPASS as this shell sees it
	EnvAskpassRequire string // SSH_ASKPASS_REQUIRE as this shell sees it
}

// AgentView is one ssh-agent process as the report presents it.
type AgentView struct {
	PID       int
	UID       int // owning uid, or -1 when it could not be read
	Kind      agent.ProcKind
	Socket    string
	Reachable bool
	Ancestry  []ProcInfo // the process chain that launched it, agent first
	Cgroup    string     // systemd unit the agent's cgroup names, or "" if none/unknown
}

// KeyView is one ~/.ssh key file as the report presents it.
type KeyView struct {
	Name        string // base filename, e.g. "id_ed25519"
	Fingerprint string // "" when ssh-keygen could not read the file
	Loaded      bool   // whether Fingerprint is currently in the agent
	Tracked     bool   // whether sshakku recorded adding this key itself
	NoExpiry    bool   // Tracked, but recorded with no expiry (lifetime 0)
	ExpiresAt   time.Time
}

// Report is the read-only picture the doctor presents.
type Report struct {
	FixedSock    string
	EnvSock      string
	EnvReachable bool
	OurUID       int
	RecordedPID  int // pid from agent.state, 0 when absent or unreadable
	Agents       []AgentView
	State        State
	Findings     []string
	LogTail      []string
	InspectErr   error // enumeration failed; the report is partial
	Keys         []KeyView
	KeysErr      error // key enumeration failed; Keys is empty
	Host         HostChecks
}

// Gather inspects the agent situation described by in and returns the report,
// reading everything through src, prober, anc, and cg so it never touches the
// real /proc or sockets in a test. A nil anc skips ancestry attribution; a nil
// cg skips the cgroup fallback used when ancestry dead-ends at init. A nil
// keys skips the ~/.ssh key/TTL section entirely. A nil host skips the
// environment-hardening section entirely (Report.Host stays its zero value,
// which Format and findings both already treat as "nothing to say"). It
// mutates nothing.
func Gather(in Inputs, src AgentSource, prober agent.Prober, anc AncestrySource, cg CgroupSource, keys *KeySource, host HostSource) Report {
	r := Report{
		FixedSock: in.FixedSock,
		EnvSock:   in.EnvSock,
		OurUID:    in.OurUID,
	}
	if in.EnvSock != "" {
		r.EnvReachable = prober.Reachable(in.EnvSock)
	}
	if st, err := agent.ReadState(in.StatePath); err == nil {
		r.RecordedPID = st.PID
	}

	procs, err := src.Agents()
	if err != nil {
		r.InspectErr = err
	}
	for _, p := range procs {
		av := AgentView{
			PID:       p.PID,
			UID:       p.UID,
			Kind:      agent.Classify(p, in.FixedSock, in.LegacyDir),
			Socket:    p.Socket,
			Reachable: p.Socket != "" && prober.Reachable(p.Socket),
			Ancestry:  ancestry(p.PID, anc),
		}
		if cg != nil {
			if unit, ok := cg.Cgroup(p.PID); ok {
				av.Cgroup = unit
			}
		}
		r.Agents = append(r.Agents, av)
	}

	r.State = classifyState(r)
	r.LogTail = tailLines(in.LogFile, logTailLines)
	if host != nil {
		r.Host = host.Checks()
	}
	r.Findings = findings(in, r)
	if keys != nil {
		r.Keys, r.KeysErr = gatherKeys(*keys)
	}
	return r
}

// gatherKeys enumerates ~/.ssh keys through ks.Lister, cross-references each
// one's fingerprint against the agent's loaded set, and — for a loaded key —
// looks up how long sshakku recorded it as living there. A nil Fingerprint or
// State collaborator degrades gracefully: fingerprints/loaded state or
// tracked/TTL info is simply left at its zero value rather than failing the
// whole report.
func gatherKeys(ks KeySource) ([]KeyView, error) {
	files, err := ks.Lister.Keys()
	if err != nil {
		return nil, err
	}

	var agentFPs map[string]bool
	if ks.Fingerprint != nil {
		agentFPs, _ = ks.Fingerprint.AgentFingerprints()
	}

	views := make([]KeyView, 0, len(files))
	for _, f := range files {
		kv := KeyView{Name: filepath.Base(f)}
		if ks.Fingerprint != nil {
			kv.Fingerprint, _ = ks.Fingerprint.FileFingerprint(f)
		}
		kv.Loaded = kv.Fingerprint != "" && agentFPs[kv.Fingerprint]
		if kv.Loaded && ks.State != nil {
			if rec, ok := ks.State.Load(kv.Name); ok {
				kv.Tracked = true
				if expiresAt, hasExpiry := rec.ExpiresAt(); hasExpiry {
					kv.ExpiresAt = expiresAt
				} else {
					kv.NoExpiry = true
				}
			}
		}
		views = append(views, kv)
	}
	return views, nil
}

// differentUser reports whether a is owned by a real uid other than the one
// this report is about. That is an ordinary multi-user fact — someone else's
// ssh-agent, visible to a privileged caller or simply coexisting on the host —
// not evidence of tampering with this report's own account. Unknown ownership
// (-1) is treated conservatively as possibly this account's, matching the rest
// of the report.
func differentUser(a AgentView, ourUID int) bool {
	return a.UID >= 0 && a.UID != ourUID
}

// orphanTokenLen is the hex length of sshakku's own per-login socket token
// (see paths.tokenByteLen*2), duplicated here rather than imported so this
// package's attribution heuristic stays a pure string check with no
// dependency on how the token is actually produced.
const orphanTokenLen = 32

// looksLikeOrphanedOurs reports whether socket has the exact shape sshakku
// itself uses for its own per-login socket — ".../sshakku/<32-hex>/agent.sock"
// — even though its token doesn't match this session's own. An agent bound
// there is far more likely a previous instance of sshakku's own agent
// (orphaned by a keyring reset, an old build, or manual testing) than a truly
// external tool that happens to reinvent the same layout, so it is worth
// saying so explicitly rather than calling it foreign to an unknown launcher.
// This is a naming-convention heuristic, not proof: it only ever changes
// wording, never reap/adopt behaviour.
func looksLikeOrphanedOurs(socket string) bool {
	if filepath.Base(socket) != "agent.sock" {
		return false
	}
	tokenDir := filepath.Dir(socket)
	token := filepath.Base(tokenDir)
	if len(token) != orphanTokenLen || !isLowerHex(token) {
		return false
	}
	return filepath.Base(filepath.Dir(tokenDir)) == "sshakku"
}

// isLowerHex reports whether s consists solely of lowercase hex digits.
func isLowerHex(s string) bool {
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// knownForeignShape identifies socket as belonging to a well-known
// ssh-agent-compatible service other than sshakku, by the fixed path shape
// each is known to bind. Unlike looksLikeOrphanedOurs, these never surface
// as an AgentView — Inspector.Agents only enumerates processes literally
// named "ssh-agent" (internal/agent/inspect.go), and none of gnome-keyring,
// gpg-agent, or a systemd-activated unit run under that name — so this is
// checked against SSH_AUTH_SOCK itself rather than the process list.
func knownForeignShape(socket string) (string, bool) {
	switch base := filepath.Base(socket); {
	case base == "S.gpg-agent.ssh":
		return "gpg-agent, with ssh support enabled", true
	case base == "ssh" && filepath.Base(filepath.Dir(socket)) == "keyring":
		return "gnome-keyring-daemon's ssh-agent emulation", true
	case base == "ssh-agent.socket":
		return "a systemd-activated ssh-agent.socket unit", true
	default:
		return "", false
	}
}

// findings turns the gathered facts into plain-language observations. It only
// describes what it sees; remediation guidance arrives with the fix path.
func findings(in Inputs, r Report) []string {
	var reachable, dead, elsewhere int
	for _, a := range r.Agents {
		switch {
		case differentUser(a, r.OurUID):
			if a.Socket != "" {
				elsewhere++
			}
		case a.Reachable:
			reachable++
		case a.Socket != "":
			dead++
		}
	}

	var f []string
	switch {
	case in.EnvSock == "":
		f = append(f, "SSH_AUTH_SOCK is unset — this shell cannot reach any agent")
	case !r.EnvReachable:
		f = append(f, fmt.Sprintf("SSH_AUTH_SOCK points at %s, which is not answering", in.EnvSock))
	case in.EnvSock != in.FixedSock:
		if label, ok := knownForeignShape(in.EnvSock); ok {
			f = append(f, fmt.Sprintf("SSH_AUTH_SOCK is %s (%s), not our fixed socket %s", in.EnvSock, label, in.FixedSock))
		} else {
			f = append(f, fmt.Sprintf("SSH_AUTH_SOCK is %s, not our fixed socket %s", in.EnvSock, in.FixedSock))
		}
	}

	switch {
	case reachable == 0:
		f = append(f, "no ssh-agent is answering; a new login shell will start one")
	case reachable > 1:
		f = append(f, fmt.Sprintf("%d agents are answering; normally only one should serve you", reachable))
	}
	if dead > 0 {
		f = append(f, fmt.Sprintf("%d dead ssh-agent process(es) with a stale socket are lingering", dead))
	}
	if elsewhere > 0 {
		f = append(f, fmt.Sprintf("%d ssh-agent process(es) belong to a different user account — visible here, but not part of this account's session", elsewhere))
	}
	for _, a := range r.Agents {
		if a.Kind != agent.KindForeign || !a.Reachable || differentUser(a, r.OurUID) {
			continue
		}
		if looksLikeOrphanedOurs(a.Socket) {
			f = append(f, fmt.Sprintf(
				"pid %d looks like a previous sshakku-managed agent (its socket has our own naming shape, but a different per-login token) rather than a truly external tool — investigate only if you don't recognize ever running sshakku here",
				a.PID))
			continue
		}
		who := "an unknown launcher"
		if label, ok := startedBy(a.Ancestry, a.Cgroup); ok {
			who = label
		}
		f = append(f, fmt.Sprintf("a foreign ssh-agent (pid %d) started by %s is answering", a.PID, who))
	}
	if r.InspectErr != nil {
		f = append(f, fmt.Sprintf("could not enumerate processes: %v (report is partial)", r.InspectErr))
	}
	if in.GUIAvailable && (in.EnvAskpass == "" || in.EnvAskpassRequire == "") {
		f = append(f, askpassNotWiredMsg)
	}
	f = append(f, hostFindings(r.Host)...)

	if len(f) == 0 {
		f = append(f, "no problems detected")
	}
	return f
}

// Format writes a human-readable rendering of r to w. It builds the whole report
// first and writes it once, so a short write cannot leave a half-printed report.
func Format(w io.Writer, r Report) {
	var b strings.Builder
	p := func(format string, a ...any) { _, _ = fmt.Fprintf(&b, format, a...) }

	p("sshakku doctor — ssh-agent diagnostics\n\n")
	p("state: %s\n\n", r.State)
	p("fixed socket:  %s\n", orNone(r.FixedSock))
	p("SSH_AUTH_SOCK: %s%s\n", orUnset(r.EnvSock), envReachSuffix(r.EnvSock, r.EnvReachable))
	if r.RecordedPID != 0 {
		p("recorded pid:  %d (agent.state)\n", r.RecordedPID)
	}

	p("\nssh-agent processes (%d):\n", len(r.Agents))
	if len(r.Agents) == 0 {
		p("  (none)\n")
	}
	for _, a := range r.Agents {
		state := "dead"
		if a.Reachable {
			state = "reachable"
		}
		p("  pid %-7d %-7s %-9s %-6s %s\n",
			a.PID, a.Kind, state, uidNote(a.UID, r.OurUID), orNone(a.Socket))
		if label, ok := startedBy(a.Ancestry, a.Cgroup); ok {
			p("    started by %s\n", label)
			p("    %s\n", chainString(a.Ancestry))
		}
	}

	if len(r.Keys) > 0 || r.KeysErr != nil {
		p("\n~/.ssh keys (%d):\n", len(r.Keys))
		for _, k := range r.Keys {
			p("  %-28s %s\n", k.Name, keyStatus(k))
		}
		if r.KeysErr != nil {
			p("  could not enumerate ~/.ssh: %v\n", r.KeysErr)
		}
	}

	if line := hostChecksLine(r.Host); line != "" {
		p("\nenvironment:\n  %s\n", line)
	}

	p("\nfindings:\n")
	for _, s := range r.Findings {
		p("  - %s\n", s)
	}

	if rec := recommend(r.State); rec != "" {
		p("\nrecommendation:\n  %s\n", rec)
	}

	if len(r.LogTail) > 0 {
		p("\nrecent log:\n")
		for _, line := range r.LogTail {
			p("  %s\n", line)
		}
	}

	_, _ = io.WriteString(w, b.String())
}

// uidNote labels an agent's owning uid, marking the invoking user's own agents.
func uidNote(uid, ourUID int) string {
	if uid < 0 {
		return "uid ?"
	}
	if uid == ourUID {
		return "you"
	}
	return "uid " + strconv.Itoa(uid)
}

// envReachSuffix annotates the SSH_AUTH_SOCK line with its reachability, and
// nothing when the variable is unset.
func envReachSuffix(sock string, reachable bool) string {
	if sock == "" {
		return ""
	}
	if reachable {
		return "  (reachable)"
	}
	return "  (not answering)"
}

// keyStatus renders one KeyView's loaded/TTL status for the report.
func keyStatus(k KeyView) string {
	if !k.Loaded {
		return "not loaded"
	}
	switch {
	case k.NoExpiry:
		return "loaded, no expiry"
	case k.Tracked:
		remaining := k.ExpiresAt.Sub(now())
		if remaining >= 0 {
			return fmt.Sprintf("loaded, expires in %s", remaining.Round(time.Second))
		}
		// sshakku's own record says this key's lifetime elapsed, yet the agent
		// still has it: our record can no longer be trusted for it (the agent
		// only drops a key exactly at its ssh-add -t deadline, so something
		// re-added or extended it since — outside sshakku's tracking, since a
		// key sshakku itself reloads would have a fresh record). A new shell
		// will not "refill" it either: the loader dedups on an already-loaded
		// fingerprint and skips it.
		return fmt.Sprintf("loaded, TTL unknown (sshakku's record expired %s ago, but the agent still has it — likely refreshed outside sshakku)", (-remaining).Round(time.Second))
	default:
		return "loaded, TTL unknown (not added by sshakku, or added before a reboot)"
	}
}

// minTmpBytes is the size below which a tmpfs /tmp is flagged as possibly
// too small; advisory only, not a hard requirement.
const minTmpBytes = 512 * 1024 * 1024

// hostFindings turns h into advisory observations about conditions outside
// sshakku's own control that materially weaken its threat model. Every line
// says so explicitly: doctor reports these, it never configures or refuses
// to run because of them. A nil field (undetermined) never produces a line.
func hostFindings(h HostChecks) []string {
	var f []string
	if h.DiskEncrypted != nil && !*h.DiskEncrypted {
		f = append(f, "the disk does not appear to be encrypted (best-effort LUKS check) — outside sshakku's control, but exposes the wallet database directly if the drive is lost, stolen, or discarded")
	}
	if h.TmpTmpfs != nil {
		switch {
		case !*h.TmpTmpfs:
			f = append(f, "/tmp is not a dedicated tmpfs mount — outside sshakku's control, temporary files may persist to disk instead of memory")
		case h.TmpSizeBytes > 0 && h.TmpSizeBytes < minTmpBytes:
			f = append(f, fmt.Sprintf("/tmp is tmpfs but only %s — outside sshakku's control, may be too small under load", humanBytes(h.TmpSizeBytes)))
		}
	}
	if h.TPMPresent != nil && !*h.TPMPresent {
		f = append(f, "no TPM device detected — outside sshakku's control, a TPM enables stronger disk-encryption key protection where supported")
	}
	return f
}

// hostChecksLine renders h as a single summary line for Format, or "" when h
// is the zero value (Gather was called with a nil HostSource).
func hostChecksLine(h HostChecks) string {
	if h.DiskEncrypted == nil && h.TmpTmpfs == nil && h.TPMPresent == nil {
		return ""
	}
	parts := []string{
		"disk encryption: " + triStateWord(h.DiskEncrypted),
	}
	switch {
	case h.TmpTmpfs == nil:
		parts = append(parts, "/tmp: undetermined")
	case !*h.TmpTmpfs:
		parts = append(parts, "/tmp: not tmpfs")
	case h.TmpSizeBytes > 0:
		parts = append(parts, fmt.Sprintf("/tmp: tmpfs, %s", humanBytes(h.TmpSizeBytes)))
	default:
		parts = append(parts, "/tmp: tmpfs, size undetermined")
	}
	if h.TPMPresent == nil {
		parts = append(parts, "TPM: undetermined")
	} else if *h.TPMPresent {
		parts = append(parts, fmt.Sprintf("TPM: present (%s)", h.TPMVersion))
	} else {
		parts = append(parts, "TPM: not detected")
	}
	return strings.Join(parts, "  |  ")
}

// triStateWord renders a *bool as doctor's report prose expects.
func triStateWord(b *bool) string {
	switch {
	case b == nil:
		return "undetermined"
	case *b:
		return "yes"
	default:
		return "no"
	}
}

// humanBytes renders n in the largest whole unit that keeps at least one
// significant digit, GiB down to KiB.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

func orUnset(s string) string {
	if s == "" {
		return "(unset)"
	}
	return s
}

// tailLines returns the last n non-empty-trailing lines of the file at path, or
// nil when the file is missing or empty. A read error is not surfaced: the log is
// a convenience, not a required input.
func tailLines(path string, n int) []string {
	if path == "" || n <= 0 {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	trimmed := strings.TrimRight(string(b), "\n")
	if trimmed == "" {
		return nil
	}
	lines := strings.Split(trimmed, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}
