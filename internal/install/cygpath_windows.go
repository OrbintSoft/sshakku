//go:build windows

package install

import "path/filepath"

// cygpathCandidates says where a POSIX-emulating environment keeps its path
// translator, relative to an interpreter belonging to that same environment,
// in the order they should be tried.
//
// Git for Windows is what this is for. It ships a second copy of the shell in
// its own `bin` beside the one in `usr\bin`, and only `usr\bin` holds the
// translator, so from `bin` it is one level up and back down.
//
// The other place is simply beside the interpreter, which is where an
// environment keeping its shell and its translator in one directory puts it.
// That is MSYS2, should it become a target, and Cygwin, which is not one.
// Neither is chased here; the second candidate costs one Stat and means a
// layout this program never had in mind is not excluded by accident.
//
// Both are tried for any interpreter rather than working out which environment
// this is: what makes a candidate the right one is that it is there.
func cygpathCandidates(interpreter string) []string {
	dir := filepath.Dir(interpreter)
	return []string{
		filepath.Join(dir, "cygpath.exe"),
		filepath.Join(filepath.Dir(dir), "usr", "bin", "cygpath.exe"),
	}
}
