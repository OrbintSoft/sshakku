//go:build windows

package install

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// bashCandidates says where this system keeps a bash that can run the shell
// library, in the order they should be tried.
//
// The name "bash" is deliberately absent. On Windows it resolves to
// C:\Windows\System32\bash.exe, which launches WSL: a real bash, but one whose
// filesystem is a different one, where this repository either is not present or
// is present under another path. Comparing against it would compare against the
// wrong machine.
//
// The bash that belongs to this machine is the one shipped with Git for
// Windows, and where Git is installed is asked of Git rather than assumed —
// `git --exec-path` reports a directory three levels below the installation
// root, so the root is what encloses it.
func bashCandidates(t *testing.T) []string {
	t.Helper()

	out, err := exec.CommandContext(t.Context(), "git", "--exec-path").Output()
	if err != nil {
		return nil
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(strings.TrimSpace(string(out)))))
	return []string{
		filepath.Join(root, "bin", "bash.exe"),
		filepath.Join(root, "usr", "bin", "bash.exe"),
	}
}
