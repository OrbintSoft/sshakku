//go:build windows

package agent

// KeepsAgents reports whether this build can keep an ssh-agent on a fixed
// endpoint on this system.
//
// Here it cannot, and the reason is the mechanism rather than unfinished
// effort: an agent on this system is a service reached through a named pipe
// rather than a process listening on a socket, and the ssh-agent that ships
// with it ignores the flag that would bind one of its own. There is no fixed
// endpoint to keep healthy, so there is nothing for the lifecycle above to
// drive — which is why callers are told once rather than left to discover it
// through whichever piece they reach first.
func KeepsAgents() bool { return false }
