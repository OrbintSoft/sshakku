package install

import "fmt"

// runningForTheMachine is this system's answer about the session running this
// command: whether it may make a change that every account on the machine will
// then run. It is held as a variable, like the other three answers above, so
// that what an install does with either answer stays checkable from a session
// that has only one of them — which is every session, since one that has the
// authority cannot drop it and one without it cannot take it.
var runningForTheMachine = haveMachineAuthority

// notThisSessionsToMakeError is a change for the whole machine asked for by a
// session that cannot finish one. It carries what was being done and what this
// system calls the authority it needs, so the sentence names both.
type notThisSessionsToMakeError struct {
	doing     string
	authority string
}

func (e notThisSessionsToMakeError) Error() string {
	return fmt.Sprintf("a machine-wide %s changes what every account on this machine runs at login,"+
		" so it has to be made by %s. Nothing has been changed: run it again from a session that has that"+
		" authority, or use --scope=%s for this account alone", e.doing, e.authority, User)
}

// refuseAChangeThisSessionCannotFinish reports the refusal owed to a change of
// the given scope, and nil where there is none.
//
// It is asked before anything is written, and that is the whole of its value.
// A machine-wide install run without the authority does not simply fail: the
// directory it writes the hook into is one the system lets any account create,
// so the unprivileged half of the work succeeds, and what fails is the half
// that needs privilege — leaving behind a directory an ordinary account owns,
// in the place the next install will find one and use it.
//
// A change for the account running the command asks for nothing, because it
// writes nowhere that account may not already write.
func refuseAChangeThisSessionCannotFinish(scope Scope, doing string) error {
	if scope != Machine || runningForTheMachine() {
		return nil
	}
	return notThisSessionsToMakeError{doing: doing, authority: machineAuthorityName}
}
