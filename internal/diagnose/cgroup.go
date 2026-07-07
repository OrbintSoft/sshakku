package diagnose

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// CgroupSource reports the systemd unit a process's cgroup names, if any.
// ProcfsCgroup is the real implementation; tests supply a fake.
type CgroupSource interface {
	Cgroup(pid int) (unit string, ok bool)
}

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
	root := c.Root
	if root == "" {
		root = "/proc"
	}
	b, err := os.ReadFile(filepath.Join(root, strconv.Itoa(pid), "cgroup"))
	if err != nil {
		return "", false
	}
	return parseCgroupUnit(b)
}

// parseCgroupUnit extracts the innermost systemd unit from a /proc/<pid>/cgroup
// file, in either the cgroup v2 unified form (a single "0::/..." line) or the
// cgroup v1 per-controller form (several "N:name=...:/..." lines). A unit is a
// path segment ending in ".service" or ".scope"; the ".slice" segments that
// contain units are never returned, since a slice is a grouping, not something
// that launches anything.
func parseCgroupUnit(b []byte) (string, bool) {
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		_, path, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		_, path, ok = strings.Cut(path, ":")
		if !ok {
			continue
		}
		segs := strings.Split(path, "/")
		for i := len(segs) - 1; i >= 0; i-- {
			if strings.HasSuffix(segs[i], ".service") || strings.HasSuffix(segs[i], ".scope") {
				return segs[i], true
			}
		}
	}
	return "", false
}
