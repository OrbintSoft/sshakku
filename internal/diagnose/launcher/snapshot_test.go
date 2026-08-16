package launcher

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A snapshot is what a system that has no per-process file to read reports its
// tree as. Taking one is that system's own business; reading one is checked
// here, which is the half that has to hold from a machine that cannot take one
// at all.
func TestASnapshotIsReadAsATreeOfParents(t *testing.T) {
	src := &SnapshotAncestry{Snapshot: table(
		ProcessEntry{PID: 100, PPID: 50, Name: "ssh-agent.exe"},
		ProcessEntry{PID: 50, PPID: 4, Name: "explorer.exe"},
		ProcessEntry{PID: 4, PPID: 0, Name: "System"},
	)}

	ppid, name, ok := src.Parent(t.Context(), 100)
	require.True(t, ok)
	assert.Equal(t, 50, ppid)
	assert.Equal(t, "ssh-agent.exe", name, "Parent answers about the process asked for, not about its parent")

	chain := Ancestry(t.Context(), 100, src)
	assert.Equal(t, []ProcInfo{{100, "ssh-agent.exe"}, {50, "explorer.exe"}, {4, "System"}}, chain)
}

func TestAProcessTheTableDoesNotNameEndsTheTrail(t *testing.T) {
	src := &SnapshotAncestry{Snapshot: table(ProcessEntry{PID: 7, PPID: 3, Name: "a.exe"})}

	_, _, ok := src.Parent(t.Context(), 999)
	assert.False(t, ok, "a process the table does not hold is not answered for")

	chain := Ancestry(t.Context(), 7, src)
	assert.Equal(t, []ProcInfo{{7, "a.exe"}}, chain,
		"the walk stops where the table stops, rather than inventing the rest")
}

// The table is taken once. A walk that re-read it at every step could be handed
// a different tree each time and assemble a chain that never existed.
func TestTheTableIsTakenOnceHoweverFarTheWalkGoes(t *testing.T) {
	taken := 0
	src := &SnapshotAncestry{Snapshot: func() ([]ProcessEntry, bool) {
		taken++
		return []ProcessEntry{
			{PID: 3, PPID: 2, Name: "c.exe"},
			{PID: 2, PPID: 1, Name: "b.exe"},
			{PID: 1, PPID: 0, Name: "a.exe"},
		}, true
	}}

	require.Len(t, Ancestry(t.Context(), 3, src), 3)

	assert.Equal(t, 1, taken, "one walk, one reading of the tree")
}

// However the table fails to arrive, nothing is attributed from it. An empty
// one and a refused one are not told apart, because nothing downstream could
// act on the difference: neither names anybody's parent.
func TestATreeThatDidNotArriveAttributesNothing(t *testing.T) {
	cases := map[string]func() ([]ProcessEntry, bool){
		"the system refused":     func() ([]ProcessEntry, bool) { return nil, false },
		"the system said naught": func() ([]ProcessEntry, bool) { return nil, true },
		"nobody supplied one":    nil,
	}

	for name, snapshot := range cases {
		t.Run(name, func(t *testing.T) {
			src := &SnapshotAncestry{Snapshot: snapshot}

			_, _, ok := src.Parent(t.Context(), 1)

			assert.False(t, ok, "a machine with no processes in it is not a thing anyone measured")
			assert.Nil(t, Ancestry(t.Context(), 1, src))
		})
	}
}

// A failure to take the table is not retried on every step of every walk: the
// answer stands for as long as the source does.
func TestATableThatCouldNotBeTakenIsNotAskedForAgain(t *testing.T) {
	asked := 0
	src := &SnapshotAncestry{Snapshot: func() ([]ProcessEntry, bool) {
		asked++
		return nil, false
	}}

	for range 3 {
		_, _, ok := src.Parent(t.Context(), 1)
		require.False(t, ok)
	}

	assert.Equal(t, 1, asked)
}

// Two rows claiming one pid contradict each other, and the reader cannot tell
// which is right. Taking the first makes the answer independent of the order
// the system happened to report them in.
func TestATableThatNamesOnePidTwiceAnswersTheSameWayEitherWay(t *testing.T) {
	first := ProcessEntry{PID: 8, PPID: 1, Name: "first.exe"}
	second := ProcessEntry{PID: 8, PPID: 2, Name: "second.exe"}

	forwards := &SnapshotAncestry{Snapshot: table(first, second)}
	ppid, name, ok := forwards.Parent(t.Context(), 8)
	require.True(t, ok)
	assert.Equal(t, 1, ppid)
	assert.Equal(t, "first.exe", name)

	backwards := &SnapshotAncestry{Snapshot: table(second, first)}
	ppid, name, ok = backwards.Parent(t.Context(), 8)
	require.True(t, ok)
	assert.Equal(t, 2, ppid, "whichever came first, and not whichever came last")
	assert.Equal(t, "second.exe", name)
}

// A report the caller has given up on must stop being assembled, and the tree
// is the expensive part of assembling it.
func TestAWalkThatWasCancelledDoesNotReadTheTree(t *testing.T) {
	taken := 0
	src := &SnapshotAncestry{Snapshot: func() ([]ProcessEntry, bool) {
		taken++
		return []ProcessEntry{{PID: 1, PPID: 0, Name: "a.exe"}}, true
	}}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, _, ok := src.Parent(ctx, 1)

	assert.False(t, ok)
	assert.Zero(t, taken, "the table is never taken for an answer nobody is waiting for")
}

func table(entries ...ProcessEntry) func() ([]ProcessEntry, bool) {
	return func() ([]ProcessEntry, bool) { return entries, true }
}
