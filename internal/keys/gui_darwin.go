//go:build darwin

package keys

import "strings"

// launchctlBin reports which kind of session a process belongs to. It is the
// only thing on macOS that answers the question at all: nothing in the
// environment distinguishes a login at the machine's own screen from an SSH
// session, so there is no equivalent of Linux's DISPLAY to read.
const launchctlBin = "launchctl"

// aquaSession is what launchctl calls a session that has a screen. The others
// it reports — Background, StandardIO, System — are sessions no window can be
// shown in: a boot into single user mode, a launchd daemon, an SSH login.
const aquaSession = "Aqua"

// GraphicalSession reports whether this process is in a session a dialog could
// appear in. Being on a Mac does not answer that: someone logged in over SSH,
// or booted into single user mode, has no window server, and a dialog raised
// there is a login shell waiting on something that can never arrive. When it
// is false the caller falls back to asking on the terminal.
func GraphicalSession(r Runner) bool {
	res, err := r.Run(Cmd{Name: launchctlBin, Args: []string{"managername"}})
	if err != nil || res.Code != 0 {
		return false
	}
	return strings.TrimSpace(string(res.Stdout)) == aquaSession
}
