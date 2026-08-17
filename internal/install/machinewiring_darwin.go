//go:build darwin

package install

import "fmt"

// machineWiringFor is this system's table of where a machine-wide wiring goes.
//
// zsh is the login shell here and reads `/etc/zprofile`; bash, for somebody who
// asks for it, reads `/etc/profile`. Neither is a directory: this system has no
// machine-wide drop-in directory its startup files loop over, so the wiring is a
// marked block inside the file — which is what the Makefile's own system-wide
// install has always written, so a machine wired by either can be unwired by the
// other.
func machineWiringFor(kind ShellKind) (machineWiring, error) {
	switch kind {
	case Zsh:
		return machineWiring{File: "/etc/zprofile"}, nil
	case Bash:
		return machineWiring{File: "/etc/profile"}, nil
	default:
		return machineWiring{}, fmt.Errorf("this system has no machine-wide startup file for a %s that an"+
			" install may assume; name the file with --profile, or install for your account with --scope=user", kind)
	}
}
