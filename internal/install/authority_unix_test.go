//go:build unix

package install

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestTheAuthorityAMachineWideChangeNeedsIsThisSystemsOwn asks this system's
// answer rather than the one handed in everywhere else, and it is the only
// place that can: the suite runs as whoever started it, and neither answer can
// be arranged for — a session holding this authority cannot drop it and one
// without it cannot take it. So what is checked is the answer owed to the
// account this run has, and that is worth checking in either direction, since a
// comparison the wrong way round is how an ordinary account gets told it may
// change the machine and finds out halfway through that it may not.
func TestTheAuthorityAMachineWideChangeNeedsIsThisSystemsOwn(t *testing.T) {
	if os.Geteuid() == 0 {
		assert.True(t, haveMachineAuthority(),
			"this run is "+machineAuthorityName+", which is the account such a change has to be made by")
		return
	}
	assert.False(t, haveMachineAuthority(),
		"this run is not "+machineAuthorityName+", and no other account holds what a machine-wide change needs")
}
