//go:build linux

package launcher

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// ProcfsCgroup reads /proc/<pid>/cgroup on a Linux procfs. Root is injectable
// for tests; empty means "/proc".
type ProcfsCgroup struct {
	Root string
}

// Cgroup returns the innermost systemd unit named in pid's /proc/<pid>/cgroup,
// or false when the file is unreadable or names no unit. Unlike process
// ancestry, cgroup membership survives a daemon's double-fork reparent to
// init, so it can still name the systemd unit (service or transient scope)
// that launched a process ancestry alone can no longer attribute.
func (c ProcfsCgroup) Cgroup(pid int) (string, bool) {
	b, err := os.ReadFile(filepath.Join(c.root(), strconv.Itoa(pid), "cgroup"))
	if err != nil {
		return "", false
	}
	return parseCgroupUnit(b)
}

// root is the procfs directory to read the cgroup files from: the real one
// unless a caller points it elsewhere. It is a method rather than an inline
// default so that the choice itself can be asserted on a machine whose own
// processes happen to belong to no systemd unit.
func (c ProcfsCgroup) root() string {
	if c.Root == "" {
		return "/proc"
	}
	return c.Root
}

// parseCgroupUnit extracts the innermost systemd unit from a /proc/<pid>/cgroup
// file, in either the cgroup v2 unified form (a single "0::/..." line) or the
// cgroup v1 per-controller form (several "N:name=...:/..." lines). A unit is a
// path segment ending in ".service" or ".scope"; the ".slice" segments that
// contain units are never returned, since a slice is a grouping, not something
// that launches anything.
func parseCgroupUnit(b []byte) (string, bool) {
	for line := range strings.SplitSeq(strings.TrimSpace(string(b)), "\n") {
		_, path, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		_, path, ok = strings.Cut(path, ":")
		if !ok {
			continue
		}
		segs := strings.Split(path, "/")
		for _, seg := range slices.Backward(segs) {
			if strings.HasSuffix(seg, ".service") || strings.HasSuffix(seg, ".scope") {
				return seg, true
			}
		}
	}
	return "", false
}
