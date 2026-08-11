//go:build windows

package agent

// UIDGatedProber wraps another Prober and reports a socket unreachable unless
// it belongs to UID. That ownership question is asked of the filesystem, which
// answers it with an SID and an ACL here rather than with a uid, so it cannot
// be answered against the uid this type is given.
//
// It therefore reports nothing reachable at all. The type exists for
// cross-user diagnosis, where the whole point is to report what the *target*
// user could reach and not what an elevated caller can bypass into: refusing
// to answer is the safe direction to be wrong in, and the wrapped Prober is
// left untouched rather than being consulted as if the gate had passed.
type UIDGatedProber struct {
	UID    int
	Prober Prober
}

// Reachable reports false.
func (UIDGatedProber) Reachable(string) bool { return false }
