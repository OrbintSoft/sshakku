package diagnose

import (
	"fmt"
	"os"
	"strings"

	"github.com/OrbintSoft/sshakku/internal/diagnose/hostcheck"
	"github.com/OrbintSoft/sshakku/internal/diagnose/launcher"

	"github.com/OrbintSoft/sshakku/internal/agent/inspect"
)

// namesTheFixedEndpoint reports whether what this shell holds names the
// endpoint sessions are pointed at. A system that writes its endpoint two ways
// hands each shell the writing that shell can carry, so both are ours and only
// something else is somebody else's.
func namesTheFixedEndpoint(in Inputs) bool {
	return in.EnvSock == in.FixedSock ||
		(in.FixedSockPosix != "" && in.EnvSock == in.FixedSockPosix)
}

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
	case in.EnvUnreadable:
		f = append(f, envUnreadableMsg)
	case in.EnvSock == "":
		f = append(f, "SSH_AUTH_SOCK is unset — this shell cannot reach any agent; "+loginShellHint)
	case !r.EnvReachable:
		f = append(f, fmt.Sprintf("SSH_AUTH_SOCK points at %s, which is not answering", in.EnvSock))
	case !namesTheFixedEndpoint(in):
		if label, ok := knownForeignShape(in.EnvSock); ok {
			f = append(f, fmt.Sprintf("SSH_AUTH_SOCK is %s (%s), not our fixed socket %s", in.EnvSock, label, in.FixedSock))
		} else {
			f = append(f, fmt.Sprintf("SSH_AUTH_SOCK is %s, not our fixed socket %s", in.EnvSock, in.FixedSock))
		}
	}

	switch {
	// Whether anything answers is asked of the endpoint too, not of the process
	// list alone: a system whose agent is a service has no process to count,
	// and counting is what would report an answering agent as none at all.
	case answersWithNoProcessToShowForIt(r):
	case reachable == 0 && r.NoAgentMechanism:
		f = append(f, "no ssh-agent is answering, and this build has no way to keep one on this system yet")
	case reachable == 0 && serviceIsDisabled(r):
		// Said below in its own words, which name what will actually help. A
		// new login shell will not start this one, and saying both would leave
		// the reader to work out which of the two sentences to believe.
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
		if a.Kind != inspect.KindForeign || !a.Reachable || differentUser(a, r.OurUID) {
			continue
		}
		if looksLikeOrphanedOurs(a.Socket) {
			f = append(f, fmt.Sprintf(
				"pid %d looks like a previous sshakku-managed agent (its socket has our own naming shape, but a different per-login token) rather than a truly external tool — investigate only if you don't recognize ever running sshakku here",
				a.PID))
			continue
		}
		who := "an unknown launcher"
		if label, ok := launcher.StartedBy(a.Ancestry, a.Cgroup); ok {
			who = label
		}
		f = append(f, fmt.Sprintf("a foreign ssh-agent (pid %d) started by %s is answering", a.PID, who))
	}
	// Where a key's lifetime is kept by the sessions rather than by the agent,
	// what it is worth differs from what was asked for, and that is worth
	// saying out loud once wherever the reader is looking — the session log
	// said it when the key was added, and this is where somebody looks
	// afterwards.
	if in.LifetimeKeptBySessions {
		f = append(f, "a key lifetime is configured, and the agent on this system holds none: "+
			"a key past its lifetime is taken out of the agent as the next session opens, "+
			"rather than at the moment it runs out")
	}
	// A service nothing may start is worth saying whether or not something is
	// answering right now: an agent still running on a machine whose service
	// has been disabled is one restart away from being gone.
	if serviceIsDisabled(r) {
		f = append(f, disabledServiceAdvice(r))
	}
	// A report is partial when something it meant to read could not be read. A
	// list this system was never going to keep is not a piece missing from the
	// report, and reported as one it would be there on every run of every
	// session, saying nothing anybody can act on.
	if r.InspectErr != nil && !keepsNoAgentProcessList(r) {
		f = append(f, fmt.Sprintf("could not enumerate processes: %v (report is partial)", r.InspectErr))
	}
	if !in.EnvUnreadable && (in.EnvAskpass == "" || in.EnvAskpassRequire == "") {
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

const minTmpBytes = 512 * 1024 * 1024

// hostFindings turns h into advisory observations about conditions outside
// sshakku's own control that materially weaken its threat model. Every line
// says so explicitly: doctor reports these, it never configures or refuses
// to run because of them. A nil field (undetermined) never produces a line.
func hostFindings(h hostcheck.Checks) []string {
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
	if h.SecureHardwarePresent != nil && !*h.SecureHardwarePresent {
		f = append(f, "no TPM or Secure Enclave detected — outside sshakku's control, that kind of hardware enables stronger disk-encryption key protection where supported")
	}
	return f
}

// hostChecksLine renders h as a single summary line for Format, or "" when h
// is the zero value (Gather was called with a nil hostcheck.Source).
func hostChecksLine(h hostcheck.Checks) string {
	if h.DiskEncrypted == nil && h.TmpTmpfs == nil && h.SecureHardwarePresent == nil {
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
		parts = append(parts, "/tmp: tmpfs, "+humanBytes(h.TmpSizeBytes))
	default:
		parts = append(parts, "/tmp: tmpfs, size undetermined")
	}
	if h.SecureHardwarePresent == nil {
		parts = append(parts, "secure hardware: undetermined")
	} else if *h.SecureHardwarePresent {
		parts = append(parts, fmt.Sprintf("secure hardware: present (%s)", h.SecureHardwareKind))
	} else {
		parts = append(parts, "secure hardware: not detected")
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

// keysDirName is the directory the keys section speaks about. A caller that
// gathered keys without saying where from leaves the report vague rather than
// naming a directory nobody read.
func keysDirName(dir string) string {
	if dir == "" {
		return "the key directory"
	}
	return dir
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
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
