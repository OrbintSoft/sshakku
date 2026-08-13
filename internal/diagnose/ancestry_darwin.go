//go:build darwin

package diagnose

import (
	"bytes"
	"context"
	"os/exec"
	"strconv"
	"time"
)

// ancestryTimeout bounds the `ps` call. Reading the process table is a plain
// status query, not something waiting on a person, so a wedged `ps` must
// surface as an unknown ancestry rather than hold up whoever asked (`doctor`).
const ancestryTimeout = 5 * time.Second

// psParent is the seam over the one shell-out, so PSAncestry's parsing is
// unit-testable without a process table to read.
var psParent = func(ctx context.Context, pid int) ([]byte, error) {
	//coverage:ignore
	ctx, cancel := context.WithTimeout(ctx, ancestryTimeout)
	//coverage:ignore
	defer cancel()
	//coverage:ignore
	return exec.CommandContext(ctx, "ps", "-o", "ppid=,comm=", "-p", strconv.Itoa(pid)).Output()
}

// PSAncestry reads the process tree with `ps`, which is how it can be read on a
// system with no procfs to walk.
type PSAncestry struct{}

// Parent returns the parent pid and short name of pid.
func (PSAncestry) Parent(ctx context.Context, pid int) (int, string, bool) {
	out, err := psParent(ctx, pid)
	if err != nil {
		return 0, "", false
	}
	return parsePS(out)
}

// parsePS reads one `ps -o ppid=,comm=` line: the parent pid, then the command,
// which is a full path when the process was started from one and may itself
// contain spaces. Everything after the first field is the name, so it is taken
// whole rather than split again.
func parsePS(out []byte) (int, string, bool) {
	line := bytes.TrimSpace(out)
	if len(line) == 0 {
		return 0, "", false
	}
	if i := bytes.IndexByte(line, '\n'); i >= 0 {
		line = bytes.TrimSpace(line[:i])
	}
	fields := bytes.SplitN(line, []byte(" "), 2)
	if len(fields) != 2 {
		return 0, "", false
	}
	ppid, err := strconv.Atoi(string(bytes.TrimSpace(fields[0])))
	if err != nil {
		return 0, "", false
	}
	// The line was trimmed before it was split, so a second field exists only
	// when there is something in it: the len check above is the whole "no name
	// here" case, and a further emptiness test could never fire.
	return ppid, string(bytes.TrimSpace(fields[1])), true
}

// reparentedLabel says what can still be told about a daemon that double-forked
// and was reparented. macOS keeps no record comparable to a Linux cgroup that
// would survive it and still name the launcher, so cgroupUnit is always empty
// here and there is nothing further to add.
func reparentedLabel(string) string {
	return "an unknown launcher (daemonized, reparented to launchd)"
}

// launcherLabel maps a known launcher's command to a friendly description.
// Unlike Linux's /proc comm, `ps -o comm=` reports the full executable path for
// anything started from one, so the bundled apps are matched by suffix.
func launcherLabel(comm string) (string, bool) {
	switch comm {
	case "/sbin/launchd", "launchd":
		return "launchd (the system or per-user manager)", true
	case "/usr/libexec/loginwindow", "loginwindow":
		return "the macOS login window", true
	case "sshd", "sshd-session":
		return "an SSH login session (sshd)", true
	case "login":
		return "a console login", true
	case "bash", "zsh", "fish", "sh", "dash", "-bash", "-zsh":
		return "a login shell (" + comm + ")", true
	}
	for path, label := range map[string]string{
		"/Terminal.app/Contents/MacOS/Terminal":           "Terminal.app",
		"/iTerm.app/Contents/MacOS/iTerm2":                "iTerm2",
		"/Ghostty.app/Contents/MacOS/ghostty":             "Ghostty",
		"/Alacritty.app/Contents/MacOS/alacritty":         "Alacritty",
		"/kitty.app/Contents/MacOS/kitty":                 "kitty",
		"/Visual Studio Code.app/Contents/MacOS/Electron": "Visual Studio Code",
	} {
		if len(comm) >= len(path) && comm[len(comm)-len(path):] == path {
			return label, true
		}
	}
	return "", false
}
