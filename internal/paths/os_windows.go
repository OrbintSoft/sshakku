//go:build windows

package paths

import (
	"os"
)

// FromOS reads the path inputs from the process environment.
//
// The XDG variables are read here as they are everywhere else: Windows has no
// convention of its own for them, so they are normally unset and Resolve falls
// back to the home-relative defaults. UID comes from os.Getuid, which is -1 on
// Windows — the value the rest of the code already reads as "unknown owner"
// rather than as a real account.
func FromOS() Env {
	return fromEnv(os.Getenv, os.UserHomeDir, os.Getuid, PrivateDir, recordedHome)
}

// recordedHome has nothing to look an account up by here: an account is a SID,
// and os.Getuid answers -1 for every process alike. "" says so, and leaves the
// home the environment named exactly as it was — which is what this build can
// honestly do until a directory can be attributed at all (see ownerUnknowable).
func recordedHome(int) string { return "" }

// ownerUnknowable: an account here is a SID and access is granted by ACL,
// neither of which PrivateDir can read yet, so no directory is turned down for
// whose it is — a question this build cannot answer must not be answered as
// though it came back "somebody else's".
const ownerUnknowable = true

// ProbeDir reports whether path is a directory meeting need. Anything beyond
// NeedThere cannot be answered here (see PrivateDir), so a caller who asks for
// it is told no.
func ProbeDir(path string, need Need) bool {
	return ProbeDirAs(os.Getuid())(path, need)
}

// ProbeDirAs is like ProbeDir for a given uid. Windows identifies an owner by
// SID and grants access by ACL, neither of which a uid can name, so every
// question about who may write or enter a directory is refused rather than
// guessed at.
func ProbeDirAs(int) func(path string, need Need) bool {
	return func(path string, need Need) bool {
		if need != NeedThere {
			return false
		}
		fi, err := os.Lstat(path)
		return err == nil && fi.IsDir()
	}
}

// PrivateDir reports whether path is a directory this user has to themselves.
// On Windows that is a question about the directory's ACL and its owner's SID:
// the permission bits Go reports for a file here are synthesised from the
// read-only attribute and say nothing about who else may enter. Until it is
// answered properly this reports false for every path, so a directory sshakku
// did not create is never treated as private — the caller then leaves it out
// rather than putting a socket somebody else may be able to wait at.
func PrivateDir(string) bool { return false }
