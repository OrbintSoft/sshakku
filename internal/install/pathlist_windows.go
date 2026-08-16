//go:build windows

package install

import "strings"

// PersistentPathList is how this system spells the list of directories in its
// PATH variable.
//
// Entries are separated by a semicolon. Two entries name the same directory
// when they differ only in the case of their letters, or in separators at the
// end — this system's file names are matched without regard to case, and a
// trailing separator names the same directory as no trailing separator.
//
// Both separators count as trailing: a value in this list may have been written
// by a program that spells paths the other way round, and it still names the
// same directory.
func PersistentPathList() PathList {
	return PathList{
		Separator: ";",
		SameEntry: func(a, b string) bool {
			return strings.EqualFold(
				trimTrailingSeparators(a, `\/`),
				trimTrailingSeparators(b, `\/`),
			)
		},
	}
}
