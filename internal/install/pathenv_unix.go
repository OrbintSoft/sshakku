//go:build unix

package install

// PathStepNothingToDo is how an install reports the step that makes the program
// runnable by name when nothing was recorded. Here that is never "it was already
// there": there is nowhere to record one at all, so a report phrased as no change
// having been needed would send somebody looking for an entry that is nobody's to
// write.
const PathStepNothingToDo = "nothing to record: this system keeps no stored environment"

// AddToPath records a directory in the search list of the given scope, which
// this system does not do.
//
// There is nowhere to record one: this system keeps no stored environment that
// outlives a session, so what a session searches is what its startup files
// built, and nothing here can add to that after the fact. Reporting that
// nothing changed is the truth about this system, not a step that was skipped —
// what makes the program findable by name here is the install that put it
// somewhere sessions already search.
func AddToPath(Scope, string) (bool, error) {
	return false, nil
}

// RemoveFromPath takes a directory back out of the search list of the given
// scope. Nothing put one there here, so there is nothing to take out — see
// AddToPath.
func RemoveFromPath(Scope, string) (bool, error) {
	return false, nil
}
