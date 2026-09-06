package keys

import "github.com/OrbintSoft/sshakku/internal/sshtools"

// SSHAddNamer names the ssh-add this session has to run to reach the agent it
// was pointed at, or reports why it has none it can.
//
// It is asked rather than given, because the answer costs a look at the
// filesystem and most sessions never need it: an account with no key to load
// runs no ssh-add, and must not be told about a program it was never going to
// run. A nil namer names the ordinary one, which is the answer on every system
// where only one build of OpenSSH can reach the agent.
type SSHAddNamer func() (string, error)

// name resolves the namer, or the ordinary program name where there is none.
func (n SSHAddNamer) name() (string, error) {
	if n == nil {
		return sshtools.SSHAddName, nil
	}
	return n()
}

// sshAddOrDefault is the program to run for a caller that named one, or the
// ordinary name for one that did not.
func sshAddOrDefault(name string) string {
	if name == "" {
		return sshtools.SSHAddName
	}
	return name
}
