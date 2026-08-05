//go:build unix

package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// FromOS reads the path inputs from the process environment.
func FromOS() Env {
	return fromEnv(os.Getenv, os.UserHomeDir, os.Getuid, PrivateDir)
}

// fromEnv is FromOS with its environment lookups injected, so the HOME fallback
// (which os.UserHomeDir also derives from $HOME, hence unreachable through the
// real os functions) can be exercised in tests.
//
// private answers whether a directory is this user's alone. It is asked here,
// at the boundary that reads the environment, so that what Resolve receives is
// already a directory we are willing to use — the environment names a
// temporary directory, and a shared one (a bare /tmp) is somewhere anybody can
// wait for a socket that is the loaded key's front door.
func fromEnv(getenv func(string) string, homeDir func() (string, error), getuid func() int, private func(string) bool) Env {
	home := getenv("HOME")
	if home == "" {
		if h, err := homeDir(); err == nil {
			home = h
		}
	}
	tempDir := getenv("TMPDIR")
	if tempDir != "" && !private(tempDir) {
		tempDir = ""
	}
	return Env{
		Home:       home,
		ConfigHome: getenv("XDG_CONFIG_HOME"),
		StateHome:  getenv("XDG_STATE_HOME"),
		RuntimeDir: getenv("XDG_RUNTIME_DIR"),
		CacheHome:  getenv("XDG_CACHE_HOME"),
		TempDir:    tempDir,
		UID:        getuid(),
	}
}

// ProbeDir reports whether path is a directory. When requireOwner is set it must
// also be owned by the current user.
func ProbeDir(path string, requireOwner bool) bool {
	return ProbeDirAs(os.Getuid())(path, requireOwner)
}

// ProbeDirAs is like ProbeDir, but for resolving another user's runtime
// directory (e.g. /run/user/<uid>) from a privileged process: it checks
// ownership against uid rather than the calling process's own, which root can
// do by simply stat'ing the path — no need to assume that uid's identity just
// to answer "is this theirs?".
func ProbeDirAs(uid int) func(path string, requireOwner bool) bool {
	return func(path string, requireOwner bool) bool {
		fi, err := os.Lstat(path)
		if err != nil || !fi.IsDir() {
			return false
		}
		if requireOwner {
			st, ok := fi.Sys().(*syscall.Stat_t)
			if !ok || int(st.Uid) != uid {
				return false
			}
		}
		return true
	}
}

// PrivateDir reports whether path is a directory this user has to themselves:
// it exists, is not a symlink, belongs to us, and grants nothing to group or
// other. It is the question to ask of a directory that was not chosen by us —
// one named by an environment variable, say — before anything of ours is put
// inside it, since a directory somebody else can write to is a directory
// somebody else can wait in.
func PrivateDir(path string) bool {
	fi, err := os.Lstat(path)
	if err != nil || !fi.IsDir() {
		return false
	}
	if fi.Mode().Perm()&0o077 != 0 {
		return false
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	return ok && int(st.Uid) == os.Getuid()
}

// Ensure creates the layout's directories (0700, leaf only) and the log file
// (0600). Intermediate parents (e.g. ~/.config) are created with the process
// umask but never re-permissioned — only our own leaf dirs are forced to 0700.
func Ensure(l Layout) error {
	for _, dir := range dedupe(l.ConfigDir, l.StateDir, l.RuntimeDir, l.SocketDir) {
		if err := ensureDir(dir, os.Chmod); err != nil {
			return err
		}
	}
	return ensureFile(l.LogFile, 0o600, os.Chmod)
}

// CleanupLegacyAgentDir retires our previous location under ~/.ssh: the agent/
// dir there is what makes OpenSSH 10.x relocate its socket to a random path.
// Best-effort: it removes only our own socket/lock and leaves the dir if
// anything else remains.
func CleanupLegacyAgentDir(home string) {
	if home == "" {
		return
	}
	dir := filepath.Join(home, ".ssh", "agent")
	if fi, err := os.Lstat(dir); err != nil || !fi.IsDir() {
		return
	}
	_ = os.Remove(filepath.Join(dir, "ssh-agent.sock"))
	_ = os.Remove(filepath.Join(dir, ".start.lock"))
	_ = os.Remove(dir) // rmdir; harmless if the dir is not empty
}

func ensureDir(path string, chmod func(string, os.FileMode) error) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	if err := chmod(path, 0o700); err != nil {
		return fmt.Errorf("chmod %s: %w", path, err)
	}
	// Reject a symlink planted in our place: the leaf must be a real directory.
	fi, err := os.Lstat(path)
	if err != nil || fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}
	return nil
}

func ensureFile(path string, perm os.FileMode, chmod func(string, os.FileMode) error) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, perm)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	_ = f.Close()
	if err := chmod(path, perm); err != nil {
		return fmt.Errorf("chmod %s: %w", path, err)
	}
	return nil
}

func dedupe(items ...string) []string {
	seen := make(map[string]bool, len(items))
	out := make([]string, 0, len(items))
	for _, it := range items {
		if !seen[it] {
			seen[it] = true
			out = append(out, it)
		}
	}
	return out
}
