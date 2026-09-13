//go:build windows

package prompt

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestThisSessionsStationIsReadFromTheSystemItself(t *testing.T) {
	station, err := thisSessionsStation()
	require.NoError(t, err, "every Windows session belongs to a window station, so reading one must succeed")

	// Read twice: a station's flags are a property of the session rather than
	// of the moment, and a call that filled the wrong bytes would not have to
	// fill the same wrong ones twice.
	again, err := thisSessionsStation()
	require.NoError(t, err, "and it must go on succeeding")
	assert.Equal(t, station, again, "a session does not gain or lose its screen between two calls")

	t.Logf("this session's window station flags: %#x (has screen: %v)", station.Flags, station.HasScreen())
}

func TestAHandleThatIsNotAStationIsRefusedRatherThanAnswered(t *testing.T) {
	// The failure a live session will not produce: the system is asked about
	// something that is not a station at all. It must come back as an error
	// rather than as a station carrying whatever was in the memory.
	_, err := stationFlags(0)
	require.Error(t, err, "a handle that names no station cannot be answered with one")

	_, err = stationFlags(^uintptr(0))
	assert.Error(t, err, "and neither can one that names nothing at all")
}

func TestASessionThatCannotBeReadIsNotASessionWithAScreen(t *testing.T) {
	original := readStation
	t.Cleanup(func() { readStation = original })
	readStation = func() (WindowStation, error) {
		return WindowStation{Flags: windowStationVisible}, errStationNotAsDescribed
	}

	assert.False(t, GraphicalSession(),
		"what could not be established is not a screen anybody has, whatever came back beside the error")
}

func TestWhetherThereIsAScreenIsWhatTheStationSays(t *testing.T) {
	// Both answers are correct here, since which one this session gets is the
	// session's own property: a developer's desk says yes, and the runner that
	// runs this suite from a service says no. What is checked is that the
	// answer is the station's rather than anything decided alongside it — a
	// second condition creeping in, or a constant put there to make a test
	// pass, and this stops agreeing.
	station, err := thisSessionsStation()
	require.NoError(t, err, "the station has to be readable for the question to mean anything")
	assert.Equal(t, station.HasScreen(), GraphicalSession(),
		"whether a dialog could appear is what this session's window station says and nothing else")
}

// F29: a process that belongs to no window station has nowhere to draw, and is
// told so rather than going on to ask about flags it has not got. No session a
// test can be run in refuses this, which is why the call is a seam and the
// refusal is held here.
func TestASessionWithNoStationAtAllIsNotAskedAboutOne(t *testing.T) {
	restore := processWindowStation
	t.Cleanup(func() { processWindowStation = restore })
	processWindowStation = func(...uintptr) (uintptr, uintptr, error) {
		return 0, 0, windows.ERROR_ACCESS_DENIED
	}

	_, err := thisSessionsStation()

	require.Error(t, err, "there is no station to read flags off, and that is the answer")
}

// The size the system says it filled is the only chance to notice that the
// struct this build declares has drifted from the one being filled. A field
// added, removed or reordered is not a compile error anywhere: it is a read of
// the wrong bytes, answered confidently, and those bytes decide whether anybody
// can see the box.
//
// The system only ever fills the size it agreed to, so this is asked of the
// half that reads the answer rather than of the call that produces it.
func TestAStationThatIsNotTheShapeThisBuildExpectsIsRefused(t *testing.T) {
	var flags userObjectFlags

	t.Run("the size the system agreed to", func(t *testing.T) {
		station, err := stationAsDescribed(userObjectFlags{Flags: 1}, uint32(unsafe.Sizeof(flags)))

		require.NoError(t, err)
		assert.Equal(t, uint32(1), station.Flags, "what was filled in is what is read back")
	})

	t.Run("any other size at all", func(t *testing.T) {
		_, err := stationAsDescribed(flags, uint32(unsafe.Sizeof(flags))-1)

		require.ErrorIs(t, err, errStationNotAsDescribed,
			"a struct that has drifted must stop the answer, not be read through anyway")
	})
}
