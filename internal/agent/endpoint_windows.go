//go:build windows

package agent

// systemPipeName is the pipe this system's own ssh-agent serves. The name is
// that program's, not a choice made here: the service binds this one name, and
// the ssh and ssh-add installed beside it reach for the same one when nothing
// in the environment says otherwise. Pointing a session anywhere else is
// pointing it at nothing.
const systemPipeName = `\\.\pipe\openssh-ssh-agent`

// SystemEndpoint is where this system keeps its agent.
func SystemEndpoint() Endpoint { return PipeEndpoint(systemPipeName) }
