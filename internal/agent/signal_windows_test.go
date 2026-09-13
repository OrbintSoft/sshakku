//go:build windows

package agent

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// F48: a graceful stop is what Terminate asks for, and this system has nothing
// to send an arbitrary process that means it.
//
// What it does offer is TerminateProcess, which is a different request: it ends
// a process where it stands rather than asking it to finish. The thing being
// ended here holds keys, so substituting the one for the other is not a near
// enough answer to make quietly — it is reported instead, and the caller decides
// whether ending an agent that way is what they meant.
func TestAGracefulStopThisSystemCannotAskForIsReportedRatherThanSubstituted(t *testing.T) {
	err := SysSignaler{}.Terminate(4242)

	require.Error(t, err,
		"there is no graceful stop to send here, and something harsher is not quietly sent in its place")
}
