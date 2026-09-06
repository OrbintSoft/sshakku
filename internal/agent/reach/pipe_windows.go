//go:build windows

package reach

import (
	"context"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// PipeProber reports whether an ssh-agent answers on a named pipe. It asks the
// same question SocketProber asks of a socket, in the same words — the agent's
// own request-identities ping — because a pipe name that opens says no more
// about the program behind it than a socket file does about the process that
// left it.
//
// The pipe is opened for overlapped I/O, and that is not an optimisation. A
// handle opened the ordinary way accepts no deadline at all ("file type does
// not support deadline"), so a read from an agent that has stopped answering
// would never come back, and a login shell would wait on it forever.
type PipeProber struct {
	// Timeout bounds open + request + response; zero means DefaultProbeTimeout.
	Timeout time.Duration
}

// setPipeDeadline applies a read/write deadline to f. It is a package variable
// so the failure path can be tested without a handle whose deadline genuinely
// cannot be set.
var setPipeDeadline = func(f *os.File, t time.Time) error { return f.SetDeadline(t) }

// Reachable reports whether an ssh-agent answers on the named pipe.
func (p PipeProber) Reachable(ctx context.Context, pipe string) bool {
	if pipe == "" {
		return false
	}
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = DefaultProbeTimeout
	}
	f, err := openPipe(pipe)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	if err := setPipeDeadline(f, time.Now().Add(timeout)); err != nil {
		return false
	}
	defer stopWaitingWhenCallerGivesUp(ctx, f)()
	return identitiesAnswered(f)
}

// stopWaitingWhenCallerGivesUp watches ctx and brings the deadline forward to
// now if it is cancelled, which is what wakes a read already waiting. It
// returns the function that ends the watch, and that function waits for the
// watcher to be gone, so nothing outlives the probe that started it.
//
// The deadline alone bounds how long the wait can be; only this ends it when
// the caller above has stopped waiting for the answer.
func stopWaitingWhenCallerGivesUp(ctx context.Context, f *os.File) func() {
	done, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		select {
		case <-ctx.Done():
			_ = setPipeDeadline(f, time.Now())
		case <-done:
		}
	}()
	return func() {
		close(done)
		<-stopped
	}
}

// openPipe opens an existing named pipe for reading and writing, in the mode
// that lets the deadline above interrupt a read, and at the only level of
// identity worth handing whatever is on the other end.
//
// A named pipe's *client* is what decides how far its server may go with the
// client's own identity, and a client that says nothing has chosen
// SecurityImpersonation: the server may call ImpersonateNamedPipeClient and
// then act as the client, with everything the client may do, for as long as the
// handle is open. The pipe namespace is the machine's and is claimed first
// come, first served, so the name an agent is expected on is one any account
// can hold while no agent has it — and this program is opened from a command
// that asks to be run by an administrator. SECURITY_IDENTIFICATION lets a
// server ask which account is calling, which agents legitimately do, and stops
// it there: anything it then tries to open as that account is refused with
// ERROR_BAD_IMPERSONATION_LEVEL.
func openPipe(name string) (*os.File, error) {
	wide, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(wide,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		0, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_OVERLAPPED|windows.SECURITY_SQOS_PRESENT|windows.SECURITY_IDENTIFICATION, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(handle), name), nil
}

var _ Prober = PipeProber{}
