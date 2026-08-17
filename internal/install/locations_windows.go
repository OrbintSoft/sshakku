//go:build windows

package install

// locationsFor is this platform's table of where an install writes.
//
// The directories are named by environment variable rather than spelled out,
// because none of them is a fixed path: a profile may be on another drive, a
// system may be installed somewhere other than C:, and the account directory is
// whatever the account was created with.
//
// A user install goes entirely under the account's own local data, so it needs
// no elevation. The program goes under `Programs`, which is where per-user
// applications are expected and where an uninstall knows to look; the rendered
// hook goes beside it rather than inside it, so a directory of executables
// stays a directory of executables.
func locationsFor(scope Scope, lookup func(string) (string, bool)) (Locations, error) {
	switch scope {
	case User:
		bin, err := directory(lookup, "LOCALAPPDATA", "Programs", "sshakku")
		if err != nil {
			return Locations{}, err
		}
		hook, err := directory(lookup, "LOCALAPPDATA", "sshakku")
		if err != nil {
			return Locations{}, err
		}
		return Locations{BinDir: bin, HookDir: hook}, nil

	case Machine:
		bin, err := directory(lookup, "ProgramFiles", "sshakku")
		if err != nil {
			return Locations{}, err
		}
		hook, err := directory(lookup, "ProgramData", "sshakku")
		if err != nil {
			return Locations{}, err
		}
		return Locations{BinDir: bin, HookDir: hook}, nil

	default:
		return Locations{}, unknownScope(scope)
	}
}
