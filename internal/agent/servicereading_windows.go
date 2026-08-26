//go:build windows

package agent

import (
	"context"
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// errServiceManagerKeptAsking is the service manager asking for a larger buffer
// and then not filling the one it asked for. Retrying again would loop, so the
// second refusal is the last.
var errServiceManagerKeptAsking = errors.New("the service manager asked twice for more room than it would then answer in")

// ReadAgentService asks this system's service manager about the service the
// agent is served from. It starts nothing: both questions it puts are
// questions, so a report built on this leaves the machine as it found it.
func ReadAgentService(ctx context.Context) ServiceReading {
	return systemService{}.read(ctx)
}

// read takes the reading — what the service is doing, and whether anything may
// start it.
//
// Whatever was learned before a refusal is kept: an account allowed to see that
// the service is stopped and not to see how it starts has told the report
// something, and dropping it would leave a reader with less than the machine
// was willing to say.
func (s systemService) read(ctx context.Context) ServiceReading {
	reading := ServiceReading{Name: s.name()}

	state, err := s.State(ctx)
	if err != nil {
		reading.Err = err
		return reading
	}
	reading.Running = state == serviceRunning

	start, err := s.startType(ctx)
	if err != nil {
		reading.Err = err
		return reading
	}
	reading.Start = start
	return reading
}

// startType asks how the service will be started when something asks for it.
//
// It opens the service for reading its configuration and for nothing else,
// which is a right an ordinary account is granted where changing that
// configuration is an administrator's alone — so this answers from the session
// a person is actually sitting in, rather than only from an elevated one.
func (s systemService) startType(ctx context.Context) (ServiceStart, error) {
	if err := ctx.Err(); err != nil {
		return ServiceStartUnknown, err
	}
	start := ServiceStartUnknown
	err := s.withService(windows.SERVICE_QUERY_CONFIG, func(handle windows.Handle) error {
		raw, err := serviceStartType(handle)
		if err != nil {
			return fmt.Errorf("asking how the %s service is started: %w", s.name(), err)
		}
		start = startTypeOf(raw)
		return nil
	})
	return start, err
}

// serviceStartType reads a service's configuration and returns the start type
// out of it, as this system's service manager spells one.
//
// The configuration is a structure followed in memory by the strings its fields
// point into, so how much room an answer needs is not known until the service
// manager has been asked. It is asked with the smallest buffer the structure
// can soundly be read out of, which is the question: a service with anything at
// all in those strings refuses that buffer and says how large one would have to
// be, and the second attempt uses the size it named. A third would mean a
// manager that asked for more than it then accepted, which is why there is not
// one rather than a loop that could go round for ever.
//
// Only the start type is taken out, while the buffer holding it is still alive.
// Nothing pointing into it outlives this call, so the strings the structure
// points at are never read through a pointer the garbage collector cannot see.
func serviceStartType(handle windows.Handle) (uint32, error) {
	size := uint32(unsafe.Sizeof(windows.QUERY_SERVICE_CONFIG{}))
	for range 2 {
		buf := make([]byte, size)
		config := (*windows.QUERY_SERVICE_CONFIG)(unsafe.Pointer(&buf[0]))
		err := windows.QueryServiceConfig(handle, config, size, &size)
		if err == nil {
			return config.StartType, nil
		}
		if !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) {
			return 0, err
		}
	}
	return 0, errServiceManagerKeptAsking
}

// startTypeOf reads the service manager's answer as one of the answers a report
// has a word for.
//
// The two that start a thing before anybody logs in are read as automatic,
// which is what they are from outside: they belong to drivers rather than to a
// service like this one, and a report that met one would be describing a
// machine arranged in a way this program has never seen, not a state it should
// leave unnamed.
func startTypeOf(raw uint32) ServiceStart {
	switch raw {
	case windows.SERVICE_AUTO_START, windows.SERVICE_BOOT_START, windows.SERVICE_SYSTEM_START:
		return ServiceStartAutomatic
	case windows.SERVICE_DEMAND_START:
		return ServiceStartOnDemand
	case windows.SERVICE_DISABLED:
		return ServiceStartDisabled
	default:
		return ServiceStartUnknown
	}
}
