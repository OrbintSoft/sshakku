package prompt

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAWindowStationSaysWhetherAnybodyCouldSeeTheWindow(t *testing.T) {
	// The second row is the one that matters and the one no developer's desk
	// can produce: every session a person opens carries the flag, and a service
	// session — a scheduled job, a runner running the suite — does not. It is
	// checked here, from whichever platform is running this test, precisely
	// because the machine that can read a real station cannot be put into that
	// state to be asked.
	for _, tc := range []struct {
		name  string
		flags uint32
		want  bool
	}{
		{"a station with a desktop behind it", windowStationVisible, true},
		{"a service's station, with none", 0, false},
		{"the flag among others", windowStationVisible | 0x0100, true},
		{"other flags and not that one", 0x0100, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, WindowStation{Flags: tc.flags}.HasScreen(),
				"whether a window would be on somebody's screen is the whole of what this decides")
		})
	}
}
