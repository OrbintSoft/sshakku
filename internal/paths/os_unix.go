//go:build unix

package paths

import (
	"os"
	"syscall"
)

// FromOS reads the path inputs from the process environment.
func FromOS() Env {
	return fromEnv(os.Getenv, os.UserHomeDir, os.Getuid, PrivateDir)
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
