//go:build linux

package install

// machineWiringFor is this system's table of where a machine-wide wiring goes.
//
// Every login shell here reads `/etc/profile.d`, one file at a time, by way of
// `/etc/profile`. A file of our own in there is also what makes the wiring
// removable without rewriting a file somebody else owns.
//
// zsh is not served, and guessing would be worse than saying so: which
// system-wide file it reads is the distribution's own choice —
// `/etc/zsh/zprofile` on some, `/etc/zprofile` on others — and a block written
// into the one this machine does not use is a wiring no session ever reads,
// reported as done. Somebody who knows theirs names it with --profile.
func machineWiringFor(kind ShellKind) (machineWiring, error) {
	if kind == Bash {
		return machineWiring{DropInDir: "/etc/profile.d"}, nil
	}
	return machineWiring{}, noMachineWiringError{kind: kind}
}
