//go:build windows

package inspect

import "errors"

// errNoProcessEnumeration is what Agents reports on Windows: the process list
// is reachable here, but a process's argv — which is how an ssh-agent is told
// apart from anything else and how its `-a <path>` is read — is not, and an
// agent on this system is a service behind a named pipe rather than a process
// bound to a socket. Reported as an error rather than as an empty list, since
// "none found" would be read as "none running" and is what a caller acts on
// when it decides to start one.
var errNoProcessEnumeration = errors.New("enumerating ssh-agent processes is not implemented on windows")

// platformAgents reports errNoProcessEnumeration.
func platformAgents() ([]AgentProc, error) { return nil, errNoProcessEnumeration }
