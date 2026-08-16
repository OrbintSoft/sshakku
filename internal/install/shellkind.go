package install

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/OrbintSoft/sshakku/internal/diagnose/launcher"
)

// ShellKind is which interpreter an install wires. It is not the same question
// as which language a hook is written in — the two PowerShell editions share a
// language and share no profile.
type ShellKind string

const (
	// Auto asks this program to work it out from the process that started it.
	Auto ShellKind = "auto"
	// Bash is a Bourne shell of the bash family, including the one a
	// POSIX-emulating environment provides.
	Bash ShellKind = "bash"
	// Zsh is the z shell.
	Zsh ShellKind = "zsh"
	// PowerShellCore is PowerShell 6 and later, the `pwsh` executable.
	PowerShellCore ShellKind = "powershellcore"
	// WindowsPowerShell is Windows PowerShell 5.x, the `powershell` executable.
	WindowsPowerShell ShellKind = "windowspowershell"
)

// shellPattern matches an interpreter by the base name of its image file.
type shellPattern struct {
	// base is the file name, without a directory.
	base string
	kind ShellKind
	// confirmByTranslator marks a name that more than one program answers to,
	// where being part of a POSIX-emulating environment is what makes this one
	// the shell meant. Where it is false the name settles it.
	confirmByTranslator bool
}

// ParseShellKind reads a `--shell` value, refusing one this system cannot wire
// rather than falling back to something it can.
//
// Which kinds a system offers is that system's own answer: there is no Windows
// PowerShell anywhere but Windows, and no zsh worth wiring under a
// POSIX-emulating environment on it.
func ParseShellKind(name string) (ShellKind, error) {
	if name == string(Auto) {
		return Auto, nil
	}
	for _, kind := range offeredShellKinds() {
		if name == string(kind) {
			return kind, nil
		}
	}
	return "", fmt.Errorf("this system cannot wire a shell called %q; it can wire %s, or %s to work it out",
		name, list(offeredShellKinds()), Auto)
}

// offeredShellKinds is the distinct kinds this system's table can recognise, in
// the order a refusal offers them.
func offeredShellKinds() []ShellKind {
	var kinds []ShellKind
	for _, pattern := range shellPatterns() {
		if !contains(kinds, pattern.kind) {
			kinds = append(kinds, pattern.kind)
		}
	}
	return kinds
}

// RecogniseShell says which shell the interpreter at imagePath is, or false
// when this system's table does not know it — or knows the name but cannot
// confirm this is the program that answers to it.
//
// The path is the whole of what is judged, and for one name that matters: on
// Windows more than one program is called bash.exe, and the one that launches
// another operating system entirely is not a shell an install can wire. The
// confirmation is positive evidence rather than a list of known-bad places: the
// shell meant is one belonging to a POSIX-emulating environment, and such an
// environment ships a path translator beside its shell. A blacklist would admit
// every impostor nobody had thought of yet.
func RecogniseShell(imagePath string) (ShellKind, bool) {
	base := strings.ToLower(filepath.Base(imagePath))
	for _, pattern := range shellPatterns() {
		if base != strings.ToLower(pattern.base) {
			continue
		}
		if pattern.confirmByTranslator {
			if _, found := FindCygpath(imagePath); !found {
				return "", false
			}
		}
		return pattern.kind, true
	}
	return "", false
}

// ImageSource reports the full path of the program a process is running, which
// is more than a process table's name: two programs of the same name are told
// apart only by where they are.
type ImageSource func(pid int) (string, bool)

// ResolveShell works out which shell to wire by reading the ancestry of the
// process that ran this command, stopping at the first ancestor it recognises.
//
// This is what makes `sshakku install`, typed in a window, wire that window's
// shell. Nothing is guessed: an ancestry that names no shell this system can
// recognise is reported as that, so the command can ask for --shell rather than
// pick one.
func ResolveShell(ctx context.Context, pid int, ancestry launcher.AncestrySource, image ImageSource) (ShellKind, string, error) {
	chain := launcher.Ancestry(ctx, pid, ancestry)
	if len(chain) == 0 {
		return "", "", fmt.Errorf("this system did not say what started process %d, so the shell has to be named with --shell", pid)
	}

	for _, process := range chain {
		// The image path when this system can give one, and the process
		// table's name when it cannot. A name alone settles every kind whose
		// pattern does not ask for confirmation, and honestly settles none of
		// the others.
		candidate := process.Name
		if image != nil {
			if path, ok := image(process.PID); ok {
				candidate = path
			}
		}
		if kind, ok := RecogniseShell(candidate); ok {
			return kind, candidate, nil
		}
	}

	return "", "", fmt.Errorf("none of %s is a shell this system can wire, so the shell has to be named with --shell",
		launcher.Chain(chain))
}

func contains(kinds []ShellKind, kind ShellKind) bool {
	for _, known := range kinds {
		if known == kind {
			return true
		}
	}
	return false
}

func list(kinds []ShellKind) string {
	names := make([]string, len(kinds))
	for i, kind := range kinds {
		names[i] = string(kind)
	}
	return strings.Join(names, ", ")
}
