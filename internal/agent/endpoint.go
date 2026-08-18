package agent

import "strings"

// An Endpoint is what a shell is pointed at to reach the agent, in every
// writing a shell may need it in.
//
// A system whose agent listens on a socket needs one writing, the path, and
// every shell reads it the same. A system that reaches its agent through a
// named pipe needs two: the one that system's own tooling reads, and the one a
// POSIX-emulating shell can carry — the environment such a shell hands to a
// native program rewrites the first on the way through, and what arrives is a
// name nothing is listening on.
type Endpoint struct {
	native string
	posix  string
}

// PipeEndpoint is the endpoint of an agent reached through the named pipe
// called name, written as that system writes it.
func PipeEndpoint(name string) Endpoint {
	return Endpoint{native: name, posix: separatorsSlashed(name)}
}

// Native is the endpoint in its own system's writing: what to hand a shell
// that belongs to that system, and what to show anyone reading a report.
func (e Endpoint) Native() string { return e.native }

// ForPosixShell is the endpoint in the writing a POSIX-emulating shell can
// carry. It names the same agent as Native does.
func (e Endpoint) ForPosixShell() string { return e.posix }

// separatorsSlashed rewrites a path's separators the other way round, which is
// what a name has to survive being carried through a POSIX-emulating shell.
// Both writings reach the same object: the system underneath accepts either.
func separatorsSlashed(s string) string { return strings.ReplaceAll(s, `\`, "/") }
