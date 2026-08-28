// Package launcher works out what started an ssh-agent, which is the difference
// between one SSHakku is tending and one that came from somewhere else — a
// desktop session, an SSH login, a service unit. `sshakku doctor` names it so a
// person looking at an agent they did not expect knows where to go and look.
//
// It walks the process tree upward from the agent, bounded and cycle-guarded so
// a racing or hostile process table cannot run the report away, and stops at the
// first ancestor this platform recognises. A daemon that double-forked has no
// such ancestor left — it was reparented to init and its launcher is gone from
// the tree — so its control-group membership is asked instead, which often still
// names the unit it was started under.
//
// Which names are worth recognising, and what a cgroup can still say, are each
// platform's own answers, and each platform's file holds only that table.
package launcher

import (
	"context"
	"strconv"
	"strings"
)

// maxAncestry bounds how far up the process tree we walk, so a pathological or
// looping process table can never make the report run away.
const maxAncestry = 8

// ProcInfo identifies one process in an agent's ancestry: its pid and the short
// name the operating system records for it.
type ProcInfo struct {
	PID  int
	Name string
}

// AncestrySource reports a process's parent pid and short name. Each platform
// supplies the real implementation for how it can be read there; tests supply a
// fake.
type AncestrySource interface {
	Parent(ctx context.Context, pid int) (ppid int, name string, ok bool)
}

// ancestry walks the process tree from pid toward pid 1, returning the chain of
// processes (the pid itself first, then each parent up to init). It is bounded in
// depth and guards against a cycle, so a hostile or racing /proc cannot loop it.
func Ancestry(ctx context.Context, pid int, src AncestrySource) []ProcInfo {
	if src == nil {
		return nil
	}
	var chain []ProcInfo
	seen := map[int]bool{}
	for cur := pid; cur >= 1 && !seen[cur] && len(chain) < maxAncestry; {
		seen[cur] = true
		ppid, name, ok := src.Parent(ctx, cur)
		if !ok {
			break
		}
		chain = append(chain, ProcInfo{PID: cur, Name: name})
		if ppid < 1 {
			break
		}
		cur = ppid
	}
	return chain
}

// startedBy attributes an agent to whoever launched it: the nearest ancestor
// (past the agent process itself) that matches a known session launcher, or the
// immediate parent's name when none is recognised. It returns false when the
// ancestry is too shallow to attribute anything. cgroupUnit is whatever the
// platform's CgroupSource found for the agent, if anything — used only as a
// fallback when ancestry dead-ends at init; pass "" when none was found.
//
// Which launchers are worth recognising by name, and what can still be said
// when the trail ends at init, are each platform's own answers: launcherLabel
// and reparentedLabel are supplied per platform.
func StartedBy(chain []ProcInfo, cgroupUnit string) (string, bool) {
	if len(chain) < 2 {
		return "", false
	}
	ancestors := chain[1:] // skip the agent process itself.
	// A daemon double-forks and is reparented to init (pid 1); its real launcher
	// is then gone from the process tree, so ancestry cannot attribute it. The
	// process's cgroup membership survives the reparent, though, and often still
	// names the systemd unit it was launched under — say that instead of only
	// "unknown" when it's available.
	if ancestors[0].PID == 1 {
		return reparentedLabel(cgroupUnit), true
	}
	for _, p := range ancestors {
		if label, known := launcherLabel(p.Name); known {
			return label, true
		}
	}
	// Nothing recognised: fall back to naming the immediate parent.
	return ancestors[0].Name, true
}

// chainString renders an ancestry chain as "name(pid) ← name(pid) ← …".
func Chain(chain []ProcInfo) string {
	parts := make([]string, len(chain))
	for i, p := range chain {
		parts[i] = p.Name + "(" + strconv.Itoa(p.PID) + ")"
	}
	return strings.Join(parts, " ← ")
}
