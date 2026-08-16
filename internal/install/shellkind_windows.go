//go:build windows

package install

import "golang.org/x/sys/windows"

// shellPatterns is this system's table of interpreters an install can wire.
//
// The two PowerShell editions are separate entries because they are separate
// targets: different executables, different profile directories, and different
// execution policies for the same account.
//
// bash.exe is the one name here that does not settle the question. The
// launcher for another operating system installed alongside this one answers
// to it as well, and wiring a hook into that would write into a filesystem
// this program cannot see, for sessions it cannot start. What tells them apart
// is that the shell meant belongs to a POSIX-emulating environment, and such
// an environment ships its path translator beside its shell.
//
// zsh is absent: no POSIX-emulating environment here ships one that an install
// has any business wiring.
func shellPatterns() []shellPattern {
	return []shellPattern{
		{base: "pwsh.exe", kind: PowerShellCore},
		{base: "powershell.exe", kind: WindowsPowerShell},
		{base: "bash.exe", kind: Bash, confirmByTranslator: true},
	}
}

// ImagePath reports the full path of the program a process is running.
//
// A process table here gives a file name without a directory, which is exactly
// what is not enough to tell two programs of the same name apart. This asks the
// system for the whole path instead, with the narrowest right to do so: the
// limited-information right exists for this question and is granted for
// processes that a full query right would be refused for.
func ImagePath(pid int) (string, bool) {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return "", false
	}
	defer func() { _ = windows.CloseHandle(handle) }()

	// The system fills the buffer and writes back how much it used. A path
	// here may exceed the traditional limit, so the buffer is the long one.
	buffer := make([]uint16, windows.MAX_LONG_PATH)
	size := uint32(len(buffer))
	if err := windows.QueryFullProcessImageName(handle, 0, &buffer[0], &size); err != nil {
		return "", false
	}
	return windows.UTF16ToString(buffer[:size]), true
}
