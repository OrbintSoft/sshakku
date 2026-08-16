package install

import (
	"fmt"
	"os"
	"path/filepath"
)

// Locations are the directories one scope's install writes to, in this
// program's own spelling.
type Locations struct {
	// BinDir is where the program itself goes.
	BinDir string
	// HookDir is where the rendered login hook goes. It is separate from
	// BinDir because the two are governed differently: a directory of
	// executables may be on PATH and audited as such, while the hook is data
	// this program renders and rewrites on every install.
	HookDir string
}

// LocationsFor returns the directories a scope's install writes to on this
// system.
func LocationsFor(scope Scope) (Locations, error) {
	return locationsFor(scope, os.LookupEnv)
}

// directory builds a path from the value of a named environment variable.
//
// A variable that is not set is an error naming it, never an empty string
// silently joined onto: that would turn `%LOCALAPPDATA%\sshakku` into a
// relative `sshakku`, which resolves against whatever directory the install was
// run from — a plausible-looking path, created without complaint, that no
// session will ever read.
func directory(lookup func(string) (string, bool), variable string, elements ...string) (string, error) {
	base, ok := lookup(variable)
	if !ok || base == "" {
		return "", fmt.Errorf("%s is not set in this environment, so there is no directory to install into", variable)
	}
	return filepath.Join(append([]string{base}, elements...)...), nil
}

// unknownScope is the answer for a scope nobody serves, shared by both
// platforms' tables so that the refusal reads the same wherever it comes from.
func unknownScope(scope Scope) error {
	return fmt.Errorf("no install locations are defined for scope %q; scope is %q or %q", scope, User, Machine)
}
