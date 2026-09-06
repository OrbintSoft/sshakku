//go:build windows

package reach

import (
	"context"
	"os"
	"slices"
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

	// trustedOwners are the accounts an endpoint may belong to for this to
	// speak on it; empty means the ones this system's own answer names.
	//
	// It is not exported, because which accounts those are is this system's
	// answer and not a caller's to widen. It exists so the refusal can be
	// exercised against a table this machine is not named in, which is the one
	// case a test cannot arrange by serving a pipe: a test can only make a pipe
	// as the account running it.
	trustedOwners []string
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
	handle, err := openPipe(pipe)
	if err != nil {
		return false
	}
	// Asked of the handle before it becomes a file, and not of the file
	// afterwards: taking a file's handle back off it hands the runtime's poller
	// over with it, and the deadline below is what the poller does.
	if !couldBeYourAgent(handle, p.trustedOwners) {
		_ = windows.CloseHandle(handle)
		return false
	}
	f := os.NewFile(uintptr(handle), pipe)
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

// couldBeYourAgent reports whether the endpoint behind f belongs to somebody
// whose agent it could be, and is asked before a byte is sent: a name anything
// on the machine can claim is not one to speak on until it is known who is
// holding it. An agent's own handshake proves only that whatever is there knows
// the protocol, which a stranger that wants your authentication would.
//
// owners is the table to judge against, empty for this system's own.
//
// Anything that cannot be answered is answered no. The refusal costs nothing a
// real agent needs: reading an object's owner takes READ_CONTROL, which comes
// with the GENERIC_READ the handle was already opened with — so an endpoint
// that opened at all is one whose owner can be read.
func couldBeYourAgent(handle windows.Handle, owners []string) bool {
	if len(owners) == 0 {
		owners = ownersWhoseAgentThisCouldBe()
	}
	owner, err := ownerOf(handle)
	if err != nil {
		return false
	}
	return slices.Contains(owners, owner)
}

// ownersWhoseAgentThisCouldBe names the accounts an endpoint on this machine
// may belong to: this one, whose own agent it would be — the account is the
// same whether or not this process was elevated — the system, which is who
// serves the agent service's endpoint, and the administrators, who could
// already have anything here. Anybody else holding the name got to it first.
func ownersWhoseAgentThisCouldBe() []string {
	owners := make([]string, 0, 3)
	if whoWeAre, err := windows.GetCurrentProcessToken().GetTokenUser(); err == nil {
		owners = append(owners, whoWeAre.User.Sid.String())
	}
	for _, known := range []windows.WELL_KNOWN_SID_TYPE{
		windows.WinLocalSystemSid, windows.WinBuiltinAdministratorsSid,
	} {
		if sid, err := windows.CreateWellKnownSid(known); err == nil {
			owners = append(owners, sid.String())
		}
	}
	return owners
}

// ownerOf reports the account that owns the object behind handle, named the way
// this system names accounts.
func ownerOf(handle windows.Handle) (string, error) {
	descriptor, err := windows.GetSecurityInfo(handle,
		windows.SE_KERNEL_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return "", err
	}
	owner, _, err := descriptor.Owner()
	if err != nil {
		return "", err
	}
	return owner.String(), nil
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
func openPipe(name string) (windows.Handle, error) {
	wide, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return windows.InvalidHandle, err
	}
	return windows.CreateFile(wide,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		0, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_OVERLAPPED|windows.SECURITY_SQOS_PRESENT|windows.SECURITY_IDENTIFICATION, 0)
}

var _ Prober = PipeProber{}
