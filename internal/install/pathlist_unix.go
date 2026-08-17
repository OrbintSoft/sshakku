//go:build unix

package install

// PersistentPathList is how this system spells the list of directories in its
// PATH variable.
//
// Entries are separated by a colon, and two of them name the same directory
// only when they are the same text, once separators at the end are discounted:
// file names here are matched exactly, and two paths differing in case are two
// directories.
//
// No install on this system writes such a list persistently — the binary goes
// somewhere already on everyone's PATH for a machine install, and the wired
// hook adds the account's own directory for a user install. This is here so
// that what the other system does with a list can be checked from here, and so
// that the difference between the two rules is written down rather than
// implied.
func PersistentPathList() PathList {
	return PathList{
		Separator: ":",
		SameEntry: func(a, b string) bool {
			return trimTrailingSeparators(a, "/") == trimTrailingSeparators(b, "/")
		},
	}
}
