//go:build windows

package agent

// KeepsAgents reports whether this build can keep an ssh-agent on a fixed
// endpoint on this system.
//
// Here it can, though not the way the socket lifecycle does. The agent is a
// service on an endpoint of the system's own — a named pipe, fixed by the
// program that installs the service — so there is no endpoint of ours to bind
// and none to keep from going stale. What can be kept is the thing that
// matters to a session: something answering there, which is what ServiceAgent
// drives.
func KeepsAgents() bool { return true }
