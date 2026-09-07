//go:build windows

package install

import "golang.org/x/sys/windows"

// machineAuthorityName is what this system calls the authority a change to what
// every account runs has to be made with.
const machineAuthorityName = "an administrator"

// haveMachineAuthority reports whether this session holds it.
//
// The question is asked of the token this process was started with rather than
// of the account's group memberships, because on this system those two answer
// differently: an administrator's ordinary session carries a token with the
// administrators group filtered out, and every write a machine-wide install
// makes would be refused by it while the account's membership still said yes.
// What matters is what this process may do now, not what the person could do
// from another prompt.
func haveMachineAuthority() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}
