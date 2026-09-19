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

// ownerUnknowable: a uid and a mode are two fields of one stat here, so every
// directory can be attributed and a doubtful one can be turned down.
const ownerUnknowable = false

// denied is the permission bits a need forbids to group and other. Which bits
// those are is this system's answer and not the question's: a mode is what
// carries the answer here.
func (n Need) denied() os.FileMode {
	if n == NeedPrivate {
		return 0o077
	}
	return 0o022
}

// ProbeDir reports whether path is a directory meeting need.
func ProbeDir(path string, need Need) bool {
	return ProbeDirAs(os.Getuid())(path, need)
}

// ProbeDirAs is like ProbeDir, but for resolving another user's runtime
// directory (e.g. /run/user/<uid>) from a privileged process: it asks about uid
// rather than the calling process's own, which root can do by simply stat'ing
// the path — no need to assume that uid's identity just to answer "is this
// theirs?".
//
// Ownership is asked alongside the mode rather than separately, because a
// caller who could ask for the owner alone would eventually ask for it in the
// place that needed both: a directory nobody else may write is no use if it is
// not ours to begin with, and one that is ours is no use if anyone may write
// in it — unlinking an entry is governed by the directory rather than by the
// entry, so what is inside being ours and mode 0700 buys nothing there.
func ProbeDirAs(uid int) func(path string, need Need) bool {
	return func(path string, need Need) bool {
		fi, err := os.Lstat(path)
		if err != nil || !fi.IsDir() {
			return false
		}
		if need == NeedThere {
			return true
		}
		if fi.Mode().Perm()&need.denied() != 0 {
			return false
		}
		st, ok := fi.Sys().(*syscall.Stat_t)
		return ok && int(st.Uid) == uid
	}
}

// PrivateDir reports whether path is a directory this user has to themselves:
// it exists, is not a symlink, belongs to us, and grants nothing to group or
// other. It is the question to ask of a directory that was not chosen by us —
// one named by an environment variable, say — before anything of ours is put
// inside it, since a directory somebody else can write to is a directory
// somebody else can wait in.
//
// It is ProbeDirAs asked about this process's own account, rather than a second
// implementation of the same question, so the two cannot come to disagree.
func PrivateDir(path string) bool {
	return ProbeDirAs(os.Getuid())(path, NeedPrivate)
}
