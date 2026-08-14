//go:build windows

package launcher

import "context"

// This build reads no process tree: what a chain would be built from — a
// procfs to walk or a `ps` to shell out to — is not here, so nothing yet
// produces the names the two functions below describe. They are what the
// attribution rule asks each platform for, and they answer for now that
// nothing can be told; the labels worth recognising belong beside the reader
// that produces them, and arrive with it.

// NoAncestry is the AncestrySource for a system whose process tree this build
// does not read. Windows keeps no procfs to walk and answers this question
// through a snapshot API instead; until that is spoken here, every process's
// parent is reported unknown rather than guessed at.
type NoAncestry struct{}

// Parent reports nothing, always.
func (NoAncestry) Parent(context.Context, int) (int, string, bool) { return 0, "", false }

var _ AncestrySource = NoAncestry{}

// reparentedLabel says what can still be told about a daemon whose launcher is
// gone from the tree: nothing here, since no tree was read.
func reparentedLabel(string) string {
	return "an unknown launcher"
}

// launcherLabel reports no known launcher.
func launcherLabel(string) (string, bool) { return "", false }
