//go:build windows

package launcher

import (
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// NewToolhelpAncestry reads the process tree from a Toolhelp32 snapshot, which
// is how this system reports it: there is no per-process file to read, and the
// whole table arrives at once or not at all.
//
// What a parent pid means here is weaker than elsewhere, and the report has to
// be read in that light. A process's parent is recorded when it is created and
// is never cleared, so a parent that has since exited leaves its pid behind —
// and pids are reused, so the ancestor named may be a stranger that inherited
// the number. Nothing in the snapshot distinguishes the two. This is why the
// walk is bounded and cycle-guarded rather than trusted to terminate.
func NewToolhelpAncestry() *SnapshotAncestry {
	return &SnapshotAncestry{Snapshot: toolhelpSnapshot}
}

// toolhelpSnapshot takes the table. It is the whole of what this file knows
// that no other machine can check.
func toolhelpSnapshot() ([]ProcessEntry, bool) {
	handle, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, false
	}
	defer func() { _ = windows.CloseHandle(handle) }()

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(handle, &entry); err != nil {
		return nil, false
	}

	var entries []ProcessEntry
	for {
		entries = append(entries, ProcessEntry{
			PID:  int(entry.ProcessID),
			PPID: int(entry.ParentProcessID),
			Name: windows.UTF16ToString(entry.ExeFile[:]),
		})
		if err := windows.Process32Next(handle, &entry); err != nil {
			break
		}
	}
	return entries, true
}

// reparentedLabel says what can still be told about a process whose launcher is
// gone from the tree. Nothing more, here: there is no record like a control
// group that outlives the parent and still names what started it, so the
// cgroup this is handed is always empty and there is nothing to add to it.
func reparentedLabel(string) string {
	return "an unknown launcher"
}

// launcherLabel maps the image name of a known launcher to a friendly
// description. The names are what a Toolhelp32 snapshot reports: the executable
// file's name with its extension, and no path.
//
// A name is all a snapshot gives, and a name is sometimes not enough: bash.exe
// is the shell shipped with Git for Windows in one installation and the WSL
// launcher in another, and those are different worlds. Where that matters, the
// process's full image path is what tells them apart, which this table cannot
// see; it says "a login shell (bash.exe)" for both, which is true of both.
func launcherLabel(image string) (string, bool) {
	// Comparison is case-insensitive because the file names are: a snapshot
	// reports whatever case the file has on disk, and "Explorer.exe" is the
	// same program as "explorer.exe".
	switch strings.ToLower(image) {
	case "explorer.exe":
		return "the Windows desktop shell (explorer)", true
	case "winlogon.exe", "userinit.exe":
		return "a Windows interactive logon", true
	case "services.exe":
		return "the service control manager", true
	case "svchost.exe":
		return "a Windows service host", true
	case "sshd.exe":
		return "an SSH login session (sshd)", true
	case "windowsterminal.exe", "openconsole.exe", "conhost.exe":
		return "a terminal window", true
	case "powershell.exe":
		return "a login shell (Windows PowerShell)", true
	case "pwsh.exe":
		return "a login shell (PowerShell)", true
	case "cmd.exe":
		return "a login shell (cmd)", true
	case "bash.exe", "sh.exe", "zsh.exe":
		return "a login shell (" + image + ")", true
	default:
		return "", false
	}
}
