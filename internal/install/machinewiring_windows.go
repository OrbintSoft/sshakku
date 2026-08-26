//go:build windows

package install

// machineWiringFor is this system's table of where a machine-wide wiring goes
// for a Bourne shell.
//
// A Bourne shell here belongs to a POSIX-emulating environment, and the
// directory named is that environment's own `/etc/profile.d` rather than
// anything on this system's drives: it is written in the shell's spelling and
// goes through that environment's own translator before this program opens it,
// so where the environment was installed is never assumed. Not the Windows
// system's `/etc`, which does not exist.
//
// A PowerShell never arrives here. Its machine-wide profile is one of the five
// the interpreter itself reports, which is a better answer than any table.
func machineWiringFor(kind ShellKind) (machineWiring, error) {
	if kind == Bash {
		return machineWiring{DropInDir: "/etc/profile.d"}, nil
	}
	return machineWiring{}, noMachineWiringError{kind: kind}
}
