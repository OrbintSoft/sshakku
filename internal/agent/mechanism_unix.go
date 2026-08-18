//go:build unix

package agent

// KeepsAgents reports whether this build can keep an ssh-agent on a fixed
// endpoint on this system.
//
// Here it can, and that is what the rest of this package drives: an ssh-agent
// is a process listening on a unix socket whose path is handed to it, so an
// endpoint can be fixed, probed, and started on.
func KeepsAgents() bool { return true }

// KeepsLifetimes reports whether the agent on this system holds a key for a
// stated time and then drops it.
//
// Here it does: `ssh-add -t` is what asks for it, and the agent drops the key
// when the time is up, which is what a configured key lifetime rests on.
func KeepsLifetimes() bool { return true }
