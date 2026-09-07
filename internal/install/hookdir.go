package install

import "os"

// makeHookDirectory makes the directory a scope's rendered hook goes in.
//
// An account's own is made the way any directory of theirs is: it sits inside
// that account's profile, where nobody else may write in the first place, so
// there is nothing to state about it. The machine's is the system's own
// business — what a directory every account reads has to permit, and what it
// must not take from the directory it sits in, is a question only the system it
// lives on can answer.
func makeHookDirectory(scope Scope, dir string) error {
	if scope == Machine {
		return makeDirectoryTheMachineShares(dir)
	}
	return os.MkdirAll(dir, hookDirMode)
}
