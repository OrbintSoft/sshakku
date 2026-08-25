package diagnose

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/OrbintSoft/sshakku/internal/diagnose/launcher"
)

func Format(w io.Writer, r Report) {
	var b strings.Builder
	p := func(format string, a ...any) { _, _ = fmt.Fprintf(&b, format, a...) }

	p("sshakku doctor — ssh-agent diagnostics\n\n")
	p("state: %s\n\n", r.State)
	p("fixed socket:  %s\n", orNone(r.FixedSock))
	p("SSH_AUTH_SOCK: %s%s\n", envValue(r.EnvSock, r.EnvUnreadable), envReachSuffix(r.EnvSock, r.EnvReachable))
	if r.RecordedPID != 0 {
		p("recorded pid:  %d (agent.state)\n", r.RecordedPID)
	}

	// What serves the endpoint, where that is a service rather than a process
	// somebody started. A system whose agent is a process on a socket names no
	// service and gets no heading for one.
	if r.AgentService.ServedByAService() {
		p("\nagent service:\n")
		p("  %-22s %s\n", r.AgentService.Name+":", serviceLine(r.AgentService))
	}

	if keepsNoAgentProcessList(r) {
		p("\nssh-agent processes:\n")
		p("  %s\n", processListNote(r))
	} else {
		p("\nssh-agent processes (%d):\n", len(r.Agents))
		if len(r.Agents) == 0 {
			p("  (none)\n")
		}
	}
	for _, a := range r.Agents {
		state := "dead"
		if a.Reachable {
			state = "reachable"
		}
		p("  pid %-7d %-7s %-9s %-6s %s\n",
			a.PID, a.Kind, state, uidNote(a.UID, r.OurUID), orNone(a.Socket))
		if label, ok := launcher.StartedBy(a.Ancestry, a.Cgroup); ok {
			p("    started by %s\n", label)
			p("    %s\n", launcher.Chain(a.Ancestry))
		}
	}

	// A directory that was read and held no key still gets the section, empty:
	// that is the case where naming the directory is worth the most, since a
	// name rule matching nothing and a directory that is not the one the user
	// meant look identical from the outside. KeysDir is set only where the keys
	// were actually looked for, which is what keeps a report that never looked
	// — one about another user, say — silent about them.
	if r.KeysDir != "" || len(r.Keys) > 0 || r.KeysErr != nil {
		p("\nkeys in %s (%d):\n", keysDirName(r.KeysDir), len(r.Keys))
		for _, k := range r.Keys {
			p("  %-28s %s\n", k.Name, keyStatus(k, r.LifetimeKeptBySessions))
		}
		if r.KeysErr != nil {
			p("  could not enumerate %s: %v\n", keysDirName(r.KeysDir), r.KeysErr)
		}
	}

	if r.Wallet.Backend != "" {
		p("\nwallet:\n")
		p("  %-22s %s\n", "backend:", walletBackendLine(r.Wallet))
		if r.Wallet.Guard != "" {
			p("  %-22s %s\n", "guarded by:", r.Wallet.Guard)
		}
		for _, req := range r.Wallet.Requirements {
			state := "found"
			switch {
			case req.Undetermined:
				state = "undetermined"
			case !req.Present:
				state = "missing"
			}
			p("  %-22s %s — %s\n", req.Name+":", state, req.Detail)
		}
	}

	if len(r.Env) > 0 || len(r.SecretEnv) > 0 {
		p("\nenvironment variables:\n")
		for _, v := range r.Env {
			p("  %-28s %s\n", v.Name+":", envValue(v.Value, r.EnvUnreadable))
		}
		for _, s := range r.SecretEnv {
			p("  %-28s %s\n", s.Name+":", secretEnvState(s.Set, r.EnvUnreadable))
		}
	}

	if line := hostChecksLine(r.Host); line != "" {
		p("\nenvironment:\n  %s\n", line)
	}

	p("\nfindings:\n")
	for _, s := range r.Findings {
		p("  - %s\n", s)
	}

	if rec := recommend(r); rec != "" {
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
// expiryIsTheSessions says the sessions here are what keep a lifetime, which is
// what an elapsed record means on such a system (see Report).
func keyStatus(k KeyView, expiryIsTheSessions bool) string {
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
		if expiryIsTheSessions {
			// Here an elapsed record and a key still in the agent are the
			// ordinary state between the moment a lifetime runs out and the
			// next session, which is what takes the key back out. Nothing is
			// wrong and nothing has to be trusted less.
			return fmt.Sprintf("loaded, its lifetime ran out %s ago — the next session takes it out of the agent", (-remaining).Round(time.Second))
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
