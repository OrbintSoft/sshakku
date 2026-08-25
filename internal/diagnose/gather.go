package diagnose

import (
	"context"
	"path/filepath"

	"github.com/OrbintSoft/sshakku/internal/agent"
	"github.com/OrbintSoft/sshakku/internal/diagnose/hostcheck"
	"github.com/OrbintSoft/sshakku/internal/diagnose/launcher"

	"github.com/OrbintSoft/sshakku/internal/agent/inspect"
)

func Gather(ctx context.Context, in Inputs, src AgentSource, prober agent.Prober, anc launcher.AncestrySource, cg launcher.CgroupSource, keys *KeySource, host hostcheck.Source) Report {
	r := Report{
		FixedSock:     in.FixedSock,
		EnvSock:       in.EnvSock,
		OurUID:        in.OurUID,
		Env:           in.Env,
		SecretEnv:     in.SecretEnv,
		EnvUnreadable: in.EnvUnreadable,

		NoAgentMechanism:       in.NoAgentMechanism,
		LifetimeKeptBySessions: in.LifetimeKeptBySessions,
		AgentService:           in.AgentService,
	}
	if in.EnvSock != "" {
		r.EnvReachable = prober.Reachable(ctx, in.EnvSock)
	}
	if in.FixedSock != "" {
		r.FixedReachable = prober.Reachable(ctx, in.FixedSock)
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
			Kind:      inspect.Classify(p, in.FixedSock, in.LegacyDir),
			Socket:    p.Socket,
			Reachable: p.Socket != "" && prober.Reachable(ctx, p.Socket),
			Ancestry:  launcher.Ancestry(ctx, p.PID, anc),
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
		r.Host = host.Checks(ctx)
	}
	r.Findings = findings(in, r)
	if keys != nil {
		r.KeysDir = keys.Dir
		r.Keys, r.KeysErr = gatherKeys(ctx, *keys)
	}
	return r
}

// gatherKeys enumerates the user's keys through ks.Lister, cross-references each
// one's fingerprint against the agent's loaded set, and — for a loaded key —
// looks up how long sshakku recorded it as living there. A nil Fingerprint or
// State collaborator degrades gracefully: fingerprints/loaded state or
// tracked/TTL info is simply left at its zero value rather than failing the
// whole report.
func gatherKeys(ctx context.Context, ks KeySource) ([]KeyView, error) {
	files, err := ks.Lister.Keys()
	if err != nil {
		return nil, err
	}

	var agentFPs map[string]bool
	if ks.Fingerprint != nil {
		agentFPs, _ = ks.Fingerprint.AgentFingerprints(ctx)
	}

	views := make([]KeyView, 0, len(files))
	for _, f := range files {
		kv := KeyView{Name: filepath.Base(f)}
		if ks.Fingerprint != nil {
			kv.Fingerprint, _ = ks.Fingerprint.FileFingerprint(ctx, f)
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
