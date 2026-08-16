//go:build unix

package agent

// KeepsAgents reports whether this build can keep an ssh-agent on a fixed
// endpoint on this system.
//
// Here it can, and that is what the rest of this package drives: an ssh-agent
// is a process listening on a unix socket whose path is handed to it, so an
// endpoint can be fixed, probed, and started on.
func KeepsAgents() bool { return true }
