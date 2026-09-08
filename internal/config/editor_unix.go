//go:build unix

package config

import "os/exec"

// platformEditors is the editor to open a file in where nobody named one.
// POSIX requires vi of every system this file is built for, which is the only
// claim worth making about an editor the user was never asked for.
var platformEditors = []string{"vi"}

// findEditor says where a program of this name can be run from, or that it
// cannot be. PATH is the whole of the answer here: a program is run by name if
// it is on PATH, and there is nowhere else a name is looked up.
func findEditor(name string) (string, bool) {
	path, err := exec.LookPath(name)
	return path, err == nil
}
