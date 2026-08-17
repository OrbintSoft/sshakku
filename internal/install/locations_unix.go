//go:build unix

package install

import "path/filepath"

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
		// One variable, read once. Both directories hang off the account's home,
		// so asking for it twice would be two chances to ask and one answer to
		// have, with a second refusal nothing could ever produce.
		home, err := directory(lookup, "HOME")
		if err != nil {
			return Locations{}, err
		}
		return Locations{
			BinDir:  filepath.Join(home, ".local", "bin"),
			HookDir: filepath.Join(home, ".local", "share", "sshakku"),
		}, nil

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
