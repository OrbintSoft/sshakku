package prompt

// windowStationVisible is WSF_VISIBLE, the one documented flag a window station
// carries. A station without it has no desktop anybody is looking at: that is
// what a service's own station is, and what a session opened to run a scheduled
// job gets. A window drawn there is drawn where nobody will ever see it, and
// waiting for an answer to it is waiting for one that cannot come.
const windowStationVisible = 0x0001

// WindowStation is what a Windows session can say about where it would draw:
// the flags its window station carries.
//
// Only Windows can read one, and reading it is all that is Windows'. What the
// flags mean is ordinary knowledge, so the deciding lives here, where a test on
// any platform can check both answers — including the one this project's
// developers cannot produce on their own desks, since every session a person
// opens has a screen and only a service's has none.
type WindowStation struct {
	Flags uint32
}

// HasScreen reports whether a window drawn on this station would be on
// somebody's screen.
func (s WindowStation) HasScreen() bool { return s.Flags&windowStationVisible != 0 }
