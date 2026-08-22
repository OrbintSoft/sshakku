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

// KeepsLifetimes reports whether the agent on this system holds a key for a
// stated time and then drops it.
//
// It does not, and the reason is again the agent rather than anything here:
// asked to add a key with a lifetime it answers `agent refused operation` and
// adds nothing at all. It also keeps what it is given in this account's own
// registry, so a key added here outlives the session, the agent and the
// reboot. Callers are told once rather than discovering it as a key that
// would not load.
func KeepsLifetimes() bool { return false }
