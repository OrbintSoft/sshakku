//go:build unix

package install

// AddToPath records a directory in the search list of the given scope, which
// this system does not do and does not need to.
//
// There is nowhere to record it and nothing to record: a machine install puts
// the program somewhere already on everyone's search list, and a user install
// has the wired hook add the account's own directory, which is a change the
// shell makes at each login rather than one stored anywhere. Reporting that
// nothing changed is the truth about this system, not a step that was skipped.
func AddToPath(Scope, string) (bool, error) {
	return false, nil
}

// RemoveFromPath takes a directory back out of the search list of the given
// scope. Nothing put one there here, so there is nothing to take out — see
// AddToPath.
func RemoveFromPath(Scope, string) (bool, error) {
	return false, nil
}
