//go:build unix

package agent

import (
	"os"
	"syscall"
)

// UIDGatedProber wraps another Prober and reports a socket unreachable unless
// it is owned by UID — even when the calling process could still connect to it
// (root bypasses socket-file permissions). It exists for cross-user diagnosis:
// root can dial anyone's agent, but that doesn't make it reachable *to the
// target user*, and a report about that user's session should reflect what
// they themselves could reach, not what an elevated caller can bypass into.
type UIDGatedProber struct {
	UID    int
	Prober Prober
}

// Reachable reports false without dialing when socket isn't owned by UID, and
// otherwise defers to the wrapped Prober.
func (g UIDGatedProber) Reachable(socket string) bool {
	if socket == "" {
		return false
	}
	fi, err := os.Lstat(socket)
	if err != nil {
		return false
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || int(st.Uid) != g.UID {
		return false
	}
	return g.Prober.Reachable(socket)
}
