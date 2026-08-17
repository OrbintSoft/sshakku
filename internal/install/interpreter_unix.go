//go:build unix

package install

import "context"

// interpreterCandidates is where to look for a shell of one kind on this
// system: on PATH, under the names the table already knows.
//
// There is nowhere else to look. A shell here is installed by the system's own
// package manager into a directory that is on every account's PATH, and one
// that is not is one the user has a reason for and can name with --shell-exe.
func interpreterCandidates(_ context.Context, kind ShellKind) []string {
	return namedInPatterns(kind)
}
