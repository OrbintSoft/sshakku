//go:build unix

package install

// shellPatterns is this system's table of interpreters an install can wire.
//
// A name settles the question here. There is one bash, and a program of that
// name is that shell; nothing on this system answers to it while being a way
// into somewhere else.
//
// Windows PowerShell is absent because it exists nowhere but Windows. pwsh is
// present because it runs here, though wiring it is not a combination this
// project tests — see docs/TEST-MATRIX.md.
func shellPatterns() []shellPattern {
	return []shellPattern{
		{base: "bash", kind: Bash},
		{base: "zsh", kind: Zsh},
		{base: "pwsh", kind: PowerShellCore},
	}
}

// ImagePath reports the full path of the program a process is running, which
// this build does not read.
//
// It is not needed here and so is not written: the names this system's table
// matches on settle the question by themselves, which is the whole reason the
// other one needs a path. A caller gets false and falls back to the name the
// process table gave, which is the right answer rather than a degraded one.
func ImagePath(int) (string, bool) {
	return "", false
}
