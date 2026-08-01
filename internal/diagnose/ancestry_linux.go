//go:build linux

package diagnose

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ProcfsAncestry reads the process tree from a Linux procfs. Root is injectable
// for tests; empty means "/proc".
type ProcfsAncestry struct {
	Root string
}

// Parent returns the parent pid and short name of pid from /proc/<pid>/stat.
func (a ProcfsAncestry) Parent(pid int) (int, string, bool) {
	root := a.Root
	if root == "" {
		root = "/proc"
	}
	b, err := os.ReadFile(filepath.Join(root, strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0, "", false
	}
	name, ppid, ok := parseStat(b)
	if !ok {
		return 0, "", false
	}
	return ppid, name, true
}

// parseStat pulls the comm and ppid from a /proc/<pid>/stat line. comm is wrapped
// in parentheses and may itself contain spaces or ')', so we split on the final
// ')': everything before it is "pid (comm", and the space-separated fields after
// it begin with the state and then the ppid.
func parseStat(b []byte) (comm string, ppid int, ok bool) {
	s := string(b)
	open := strings.IndexByte(s, '(')
	end := strings.LastIndexByte(s, ')')
	if open < 0 || end < open {
		return "", 0, false
	}
	comm = s[open+1 : end]
	fields := strings.Fields(s[end+1:])
	if len(fields) < 2 {
		return comm, 0, false
	}
	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return comm, 0, false
	}
	return comm, ppid, true
}

// reparentedLabel says what can still be told about a daemon that double-forked
// and was reparented to init. Its cgroup membership survives that reparent and
// often still names the systemd unit it was launched under, so say that instead
// of only "unknown" when it is there.
func reparentedLabel(cgroupUnit string) string {
	if cgroupUnit != "" {
		return fmt.Sprintf("an unknown launcher (daemonized, reparented to init; systemd unit: %s)", cgroupUnit)
	}
	return "an unknown launcher (daemonized, reparented to init)"
}

// launcherLabel maps a known launcher's short (15-byte-truncated) comm to a
// friendly description. The truncated forms are what /proc actually reports.
func launcherLabel(comm string) (string, bool) {
	switch comm {
	case "systemd":
		return "systemd (user or system manager)", true
	case "gnome-keyring-d":
		return "gnome-keyring-daemon", true
	case "plasmashell", "ksmserver", "kwin_wayland", "kwin_x11", "startplasma-wa", "startplasma-x11":
		return "the KDE Plasma session", true
	case "gdm", "gdm-session-wor", "gdm-x-session", "gdm-wayland-ses":
		return "the GNOME display manager (GDM)", true
	case "sddm", "sddm-helper", "sddm-greeter":
		return "the SDDM display manager", true
	case "lightdm":
		return "the LightDM display manager", true
	case "sshd", "sshd-session":
		return "an SSH login session (sshd)", true
	case "login":
		return "a console login", true
	case "bash", "zsh", "fish", "sh", "dash":
		return "a login shell (" + comm + ")", true
	default:
		return "", false
	}
}
