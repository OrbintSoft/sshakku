//go:build windows

package prompt

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// uoiFlags is UOI_FLAGS: the index that asks a user object for its
// USEROBJECTFLAGS rather than for its name or its type.
const uoiFlags = 1

// errStationNotAsDescribed is the system filling a different number of bytes
// than the struct below claims to be. It is a value a caller can match on, since
// what follows from it — asking somewhere a person can answer — is the same
// thing that follows from having no screen, and only the reason differs.
var errStationNotAsDescribed = errors.New("the window station was not the shape this build expects")

var (
	user32                       = windows.NewLazySystemDLL("user32.dll")
	procGetProcessWindowStation  = user32.NewProc("GetProcessWindowStation")
	procGetUserObjectInformation = user32.NewProc("GetUserObjectInformationW")
)

// userObjectFlags mirrors USEROBJECTFLAGS field for field and in order. Its
// layout is what the call fills in directly, so a field added, removed or
// reordered here is not a compile error anywhere — it is a read of the wrong
// bytes, and the bytes in question decide whether anybody can see the window.
type userObjectFlags struct {
	Inherit  int32
	Reserved int32
	Flags    uint32
}

// thisSessionsStation reads the window station this process belongs to.
//
// The error from a syscall is only worth reading once the return value says the
// call failed: it carries the last error either way, and after a call that
// succeeded that is whatever happened before it.
func thisSessionsStation() (WindowStation, error) {
	station, _, err := procGetProcessWindowStation.Call()
	if station == 0 {
		return WindowStation{}, err
	}
	return stationFlags(station)
}

// stationFlags reads one station's USEROBJECTFLAGS.
//
// It takes the handle rather than fetching it so that the refusal stays
// reachable: a handle that is not a station is something a test can hand over,
// where a process belonging to no station at all is not something a test can
// arrange.
func stationFlags(station uintptr) (WindowStation, error) {
	var flags userObjectFlags
	var needed uint32
	r, _, err := procGetUserObjectInformation.Call(
		station,
		uoiFlags,
		uintptr(unsafe.Pointer(&flags)),
		unsafe.Sizeof(flags),
		uintptr(unsafe.Pointer(&needed)),
	)
	if r == 0 {
		return WindowStation{}, err
	}
	// The system says how much it filled in, and that is the only chance to
	// notice that the struct above no longer matches the one it was filling:
	// a field added, removed or reordered reads the wrong bytes and answers
	// confidently with them, and the bytes in question decide whether anybody
	// can see the window.
	if needed != uint32(unsafe.Sizeof(flags)) {
		return WindowStation{}, fmt.Errorf("%w: it filled %d bytes where USEROBJECTFLAGS is %d",
			errStationNotAsDescribed, needed, unsafe.Sizeof(flags))
	}
	return WindowStation{Flags: flags.Flags}, nil
}

// readStation is how the session is asked about itself; it is a variable so a
// test can hand back the refusal a live station will not produce on request.
var readStation = thisSessionsStation

// GraphicalSession reports whether this process is in a session a dialog could
// appear in. When it is false the caller asks somewhere else.
//
// Being on Windows does not answer that, any more than being on a Mac does. A
// session serving a scheduled job, or a service, has a window station of its own
// with no desktop behind it: a window raised there is one nobody can answer, and
// the shell that raised it would wait out its whole budget for a keystroke that
// was never going to come. A station that cannot even be read is treated the
// same way, since what cannot be established is not a screen anybody has.
func GraphicalSession() bool {
	station, err := readStation()
	if err != nil {
		return false
	}
	return station.HasScreen()
}
