//go:build windows

package install

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
)

// interpreterCandidates is where to look for a shell of one kind on this
// system.
//
// The PowerShell editions are on PATH under their own names and need nothing
// more. The Bourne shell needs a great deal more, and the two reasons pull in
// opposite directions: the shell that ships with Git is installed in a
// directory that is deliberately kept off PATH, so looking there is the only
// way to find it; while the name bash.exe *is* on PATH on a stock machine,
// belonging to the launcher for another operating system, which is not a shell
// this program can wire. So Git's own shell is looked for first and by path,
// and the bare name is left in the list for the environments that do put one
// there — MSYS2 and Cygwin — where RecogniseShell is what decides.
func interpreterCandidates(ctx context.Context, kind ShellKind) []string {
	var candidates []string
	if kind == Bash {
		candidates = append(candidates, gitBashCandidates(ctx)...)
	}
	return append(candidates, namedInPatterns(kind)...)
}

// gitBashCandidates asks Git where it keeps its own helper programs and derives
// from that the shell installed beside them.
//
// Git is asked rather than a directory being assumed, because it can be
// installed anywhere — a portable copy on a memory stick, a per-user install
// under the profile directory, a machine whose program files are on another
// drive — and it is asked with a question it answers by design.
//
// Both layouts are offered because the shell has lived in both: `bin\bash.exe`
// is the wrapper meant to be run from outside, `usr\bin\bash.exe` the shell
// itself. Git that is not installed at all simply yields nothing to try.
func gitBashCandidates(ctx context.Context) []string {
	out, err := exec.CommandContext(ctx, "git", "--exec-path").Output()
	if err != nil {
		return nil
	}
	// The answer is <root>\mingw64\libexec\git-core, three levels below the
	// installation root everything else here hangs off.
	root := filepath.Dir(filepath.Dir(filepath.Dir(strings.TrimSpace(string(out)))))
	if root == "" || root == "." {
		return nil
	}
	return []string{
		filepath.Join(root, "bin", "bash.exe"),
		filepath.Join(root, "usr", "bin", "bash.exe"),
	}
}
