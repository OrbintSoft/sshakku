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

// ProbeDir reports whether path is a directory. When requirePrivate is set it
// must also be one the current user has to themselves.
func ProbeDir(path string, requirePrivate bool) bool {
	return ProbeDirAs(os.Getuid())(path, requirePrivate)
}

// ProbeDirAs is like ProbeDir, but for resolving another user's runtime
// directory (e.g. /run/user/<uid>) from a privileged process: it asks about uid
// rather than the calling process's own, which root can do by simply stat'ing
// the path — no need to assume that uid's identity just to answer "is this
// theirs?".
//
// Having it to themselves means more than owning it. A directory its owner has
// opened to the group or to everyone is one somebody else can create in and
// rename in, and unlinking an entry is governed by the directory rather than by
// the entry, so what is inside being ours and mode 0700 buys nothing there.
// That is why both halves are one question and not two: a caller who could ask
// for the owner alone would eventually ask for it in the place that needed
// both.
func ProbeDirAs(uid int) func(path string, requirePrivate bool) bool {
	return func(path string, requirePrivate bool) bool {
		fi, err := os.Lstat(path)
		if err != nil || !fi.IsDir() {
			return false
		}
		if requirePrivate {
			if fi.Mode().Perm()&0o077 != 0 {
				return false
			}
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
//
// It is ProbeDirAs asked about this process's own account, rather than a second
// implementation of the same question, so the two cannot come to disagree.
func PrivateDir(path string) bool {
	return ProbeDirAs(os.Getuid())(path, true)
}
