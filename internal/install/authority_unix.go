//go:build unix

package install

import "os"

// machineAuthorityName is what this system calls the authority a change to what
// every account runs has to be made with.
const machineAuthorityName = "root"

// haveMachineAuthority reports whether this session holds it: the effective
// user is the one the directories a machine-wide install writes to belong to.
func haveMachineAuthority() bool {
	return os.Geteuid() == 0
}
