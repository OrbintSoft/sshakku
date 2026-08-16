//go:build unix

package install

// locationsFor is this platform's table of where an install writes.
//
// These are the directories the Makefile's install targets already use, and
// they are here so that a wiring done by this program lands where a wiring done
// by `make install` would. A machine install goes under a prefix that is on
// everyone's PATH already, which is why this platform needs no PATH entry
// recorded anywhere and the other one does.
func locationsFor(scope Scope, lookup func(string) (string, bool)) (Locations, error) {
	switch scope {
	case User:
		bin, err := directory(lookup, "HOME", ".local", "bin")
		if err != nil {
			return Locations{}, err
		}
		hook, err := directory(lookup, "HOME", ".local", "share", "sshakku")
		if err != nil {
			return Locations{}, err
		}
		return Locations{BinDir: bin, HookDir: hook}, nil

	case Machine:
		// Not from the environment: the prefix a machine install uses is a
		// property of the system's layout, not of whoever happens to be running
		// the command, and reading it from a variable would let an environment
		// redirect a machine-wide install.
		return Locations{BinDir: "/usr/local/bin", HookDir: "/usr/local/share/sshakku"}, nil

	default:
		return Locations{}, unknownScope(scope)
	}
}
