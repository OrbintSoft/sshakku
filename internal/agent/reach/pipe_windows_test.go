//go:build windows

package reach

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"golang.org/x/sys/windows"

	"github.com/OrbintSoft/sshakku/internal/agent"
)

// errNoDeadlineHere is the failure this test hands its seam, standing for a real one the
// code under test cannot be made to produce on demand.
var errNoDeadlineHere = errors.New("no deadline here")

// pipeName is a name no other test and no other run is using, since the pipe
// namespace is the machine's rather than this process's.
func pipeName(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf(`\\.\pipe\sshakku-test-%d-%s`,
		os.Getpid(), strings.NewReplacer("/", "-", " ", "-").Replace(t.Name()))
}

// fakeAgentPipe serves one connection on a named pipe of its own, handing it to
// reply, and returns the pipe's name.
//
// reply is given the file rather than a stream, because some of what a pipe's
// server can do to its client — asking to become them — is asked of the handle
// and not of the bytes.
//
// Like the socket fake above it asserts nothing: it runs on a goroutine of its
// own, where an assertion would report from outside the test's goroutine. What
// it serves is the subject's input, not its verdict.
func fakeAgentPipe(t *testing.T, reply func(*os.File)) string {
	t.Helper()
	return fakeAgentPipeNamed(t, pipeName(t), reply)
}

// fakeAgentPipeNamed is fakeAgentPipe on a name somebody else chose, for the
// one case where the name has to be agreed on before the server exists.
func fakeAgentPipeNamed(t *testing.T, name string, reply func(*os.File)) string {
	t.Helper()
	wide, err := windows.UTF16PtrFromString(name)
	require.NoError(t, err, "pipe name")
	handle, err := windows.CreateNamedPipe(wide,
		windows.PIPE_ACCESS_DUPLEX,
		windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT,
		1, 4096, 4096, 0, nil)
	require.NoError(t, err, "create pipe")
	f := os.NewFile(uintptr(handle), name)

	served := make(chan struct{})
	go func() {
		defer close(served)
		// A client that got in before the wait started is a connection, not a
		// failure, and this is the shape that says so.
		if err := windows.ConnectNamedPipe(windows.Handle(f.Fd()), nil); err != nil &&
			!errors.Is(err, windows.ERROR_PIPE_CONNECTED) {
			return
		}
		reply(f)
	}()

	t.Cleanup(func() {
		select {
		case <-served:
		case <-time.After(2 * time.Second):
			// Nobody came, and the wait for a client does not end on its own:
			// knock once so the goroutine can finish and the suite can end.
			if knock, err := openPipe(name); err == nil {
				_ = windows.CloseHandle(knock)
			}
			<-served
		}
		_ = f.Close()
	})
	return name
}

// pipeReplyIdentities answers a request with an identities-answer listing nkeys
// keys; the keys themselves are omitted, since the prober reads only the type.
func pipeReplyIdentities(nkeys uint32) func(*os.File) {
	return func(rw *os.File) {
		drainPipeRequest(rw)
		writeIdentitiesAnswer(rw, nkeys)
	}
}

// writeIdentitiesAnswer is the answer itself, for a server that has already
// read the request and had something to do in between.
func writeIdentitiesAnswer(rw io.Writer, nkeys uint32) {
	payload := []byte{msgIdentitiesAnswer, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(payload[1:], nkeys)
	frame := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(frame, uint32(len(payload)))
	copy(frame[4:], payload)
	_, _ = rw.Write(frame)
}

// procImpersonateNamedPipeClient is how a pipe's server asks to become its
// client. It is what the client's own choice of impersonation level decides the
// answer to, and there is no wrapper for it in golang.org/x/sys/windows.
var procImpersonateNamedPipeClient = windows.NewLazySystemDLL("advapi32.dll").
	NewProc("ImpersonateNamedPipeClient")

// impersonationOffer is what a pipe's server got when it asked to become its
// client: how far it may go with that client's identity, whether it could then
// open anything as them, and — where it could not ask at all — what it was
// told instead. It travels as JSON, because the half that measures it cannot be
// in this process (see the test that reads it).
type impersonationOffer struct {
	Level     uint32 `json:"level"`
	ActedAsUs bool   `json:"acted_as_us"`
	Refused   string `json:"refused,omitempty"`
}

// pipeReportImpersonationLevel answers as an agent does, and on the way asks to
// become whoever is asking, reporting how far it was allowed to go. The channel
// wants a buffer: it is read after the probe has finished, and a server blocked
// here would never send the reply the probe is waiting for.
func pipeReportImpersonationLevel(offers chan<- impersonationOffer) func(*os.File) {
	return func(f *os.File) {
		drainPipeRequest(f)
		offers <- whatBecomingTheClientWouldAllow(windows.Handle(f.Fd()))
		writeIdentitiesAnswer(f, 1)
		// Then stay until the client has gone: closing a pipe with a reply
		// still unread in it throws the reply away, and this client is in
		// another process and not held up by anything this one does.
		var whenTheClientCloses [1]byte
		_, _ = f.Read(whenTheClientCloses[:])
	}
}

// whatBecomingTheClientWouldAllow impersonates the client on the other end of
// pipe and reports the identity it was handed.
//
// Impersonation belongs to the OS thread rather than to the goroutine, so the
// thread is pinned for as long as it is somebody else: without that, the
// identity could be put down on one thread while another carries on wearing it.
func whatBecomingTheClientWouldAllow(pipe windows.Handle) impersonationOffer {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if ok, _, err := procImpersonateNamedPipeClient.Call(uintptr(pipe)); ok == 0 {
		return impersonationOffer{Refused: "asking to become the client: " + err.Error()}
	}
	defer func() { _ = windows.RevertToSelf() }()

	// Opened as this process rather than as the identity just put on, since
	// that identity is the very thing being asked about.
	var borrowed windows.Token
	if err := windows.OpenThreadToken(windows.CurrentThread(), windows.TOKEN_QUERY, true, &borrowed); err != nil {
		return impersonationOffer{Refused: "the identity it was handed: " + err.Error()}
	}
	defer func() { _ = borrowed.Close() }()

	var level, written uint32
	if err := windows.GetTokenInformation(borrowed, windows.TokenImpersonationLevel,
		(*byte)(unsafe.Pointer(&level)), uint32(unsafe.Sizeof(level)), &written); err != nil {
		return impersonationOffer{Refused: "how far that identity goes: " + err.Error()}
	}
	return impersonationOffer{Level: level, ActedAsUs: somethingCanBeOpenedAsTheClient()}
}

// somethingCanBeOpenedAsTheClient reports whether the identity this thread is
// wearing can be used to open anything at all, which is the difference the
// level decides: one can be read, the other can be used. Called while the
// identity is still on, so the answer is about it rather than about us.
func somethingCanBeOpenedAsTheClient() bool {
	self, err := windows.UTF16PtrFromString(os.Args[0])
	if err != nil {
		return false
	}
	opened, err := windows.CreateFile(self, windows.GENERIC_READ, windows.FILE_SHARE_READ,
		nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return false
	}
	_ = windows.CloseHandle(opened)
	return true
}

// drainPipeRequest reads one framed request so the prober's write completes.
func drainPipeRequest(rw io.ReadWriter) {
	var hdr [4]byte
	if _, err := io.ReadFull(rw, hdr[:]); err != nil {
		return
	}
	_, _ = io.CopyN(io.Discard, rw, int64(binary.BigEndian.Uint32(hdr[:])))
}

// F50: the endpoint a shell is pointed at is a pipe, and what makes it worth
// pointing at is that an agent answers on it. An agent holding no keys is as
// healthy as one holding several — `ssh-add -l` says as much.
func TestPipeProberReachesAnAgentThatAnswers(t *testing.T) {
	p := PipeProber{Timeout: 2 * time.Second}

	t.Run("holding keys", func(t *testing.T) {
		assert.True(t, p.Reachable(t.Context(), fakeAgentPipe(t, pipeReplyIdentities(2))),
			"an agent holding keys answers, and is reachable")
	})
	t.Run("holding none", func(t *testing.T) {
		assert.True(t, p.Reachable(t.Context(), fakeAgentPipe(t, pipeReplyIdentities(0))),
			"an empty agent is a healthy agent")
	})
}

// A pipe that opens says nothing about the program behind it: something else
// listening under that name is not an agent, and must not be reported as one.
func TestPipeProberRefusesWhatIsNotAnAgent(t *testing.T) {
	p := PipeProber{Timeout: 2 * time.Second}

	t.Run("a stranger on the line", func(t *testing.T) {
		wrongType := func(rw *os.File) {
			drainPipeRequest(rw)
			_, _ = rw.Write([]byte{0, 0, 0, 1, 99})
		}
		assert.False(t, p.Reachable(t.Context(), fakeAgentPipe(t, wrongType)),
			"an answer that is not an identities answer is not an agent")
	})
	t.Run("nobody there", func(t *testing.T) {
		assert.False(t, p.Reachable(t.Context(), pipeName(t)),
			"a name nothing is serving is not reachable")
	})
	t.Run("no name at all", func(t *testing.T) {
		assert.False(t, p.Reachable(t.Context(), ""), "there is nothing to dial")
	})
}

// F21: nothing SSHakku waits on may hold a shell up with no end. An agent that
// took the request and then said nothing is exactly the state a login must come
// back from, and the deadline is what brings it back.
func TestPipeProberGivesUpOnAnAgentThatNeverAnswers(t *testing.T) {
	silent := func(rw *os.File) {
		drainPipeRequest(rw)
		// and then nothing, until the client gives up and closes.
		var one [1]byte
		_, _ = rw.Read(one[:])
	}
	p := PipeProber{Timeout: 300 * time.Millisecond}

	start := time.Now()
	reachable := p.Reachable(t.Context(), fakeAgentPipe(t, silent))
	elapsed := time.Since(start)

	assert.False(t, reachable, "an agent that never answers is not reachable")
	assert.Less(t, elapsed, 3*time.Second, "the wait ended on the deadline rather than outliving the login")
}

// Rule 28's half of the same promise: a deadline ends the wait, a cancelled
// context ends the work. A caller who has given up must not be waited on.
func TestPipeProberStopsWhenTheCallerHasGoneAway(t *testing.T) {
	silent := func(rw *os.File) {
		drainPipeRequest(rw)
		var one [1]byte
		_, _ = rw.Read(one[:])
	}
	name := fakeAgentPipe(t, silent)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	p := PipeProber{Timeout: time.Minute}
	start := time.Now()
	reachable := p.Reachable(ctx, name)
	elapsed := time.Since(start)

	assert.False(t, reachable, "a cancelled probe reports nothing reachable")
	assert.Less(t, elapsed, 5*time.Second, "cancelling ended the read rather than waiting out the timeout")
}

// The deadline is what keeps the read interruptible, so a handle that will not
// take one is not one to read from: better no answer than an unbounded wait.
func TestPipeProberRefusesAHandleThatTakesNoDeadline(t *testing.T) {
	original := setPipeDeadline
	t.Cleanup(func() { setPipeDeadline = original })
	setPipeDeadline = func(*os.File, time.Time) error { return errNoDeadlineHere }

	p := PipeProber{Timeout: 2 * time.Second}
	assert.False(t, p.Reachable(t.Context(), fakeAgentPipe(t, pipeReplyIdentities(1))),
		"a read that could not be bounded is not one to make")
}

// pipeServerEnv names the pipe a re-executed copy of this test binary is to
// serve, and pipeReportEnv the file it writes down what its client offered it.
const (
	pipeServerEnv = "SSHAKKU_TEST_PIPE_SERVER"
	pipeReportEnv = "SSHAKKU_TEST_PIPE_REPORT"
)

// readyBeside names the file the serving half makes once its pipe exists, so
// the probe is not made before there is anything there to answer it.
func readyBeside(report string) string { return report + ".ready" }

// F58: the endpoint is a name anything on this machine could have claimed, and
// the client of a named pipe is the one that decides how far its server may go
// with the client's own identity. A client that says nothing has decided that
// the server may *be* it, with whatever privileges that client was run with —
// and doctor is the part of this program meant to be run by an administrator.
//
// The server has to be another process. Windows refuses to let a process
// impersonate a client of its own — "the parameter is incorrect", whatever the
// client offered — so a fake server in this one could not tell the two offers
// apart, and would report the dangerous case as the safe one.
func TestTheAgentsEndpointIsOpenedSoItsServerCannotActAsUs(t *testing.T) {
	if pipe := os.Getenv(pipeServerEnv); pipe != "" {
		serveAndWriteDownWhatWasOffered(t, pipe, os.Getenv(pipeReportEnv))
		return
	}

	pipe, report := pipeName(t), filepath.Join(t.TempDir(), "offer.json")
	server := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^"+t.Name()+"$")
	server.Env = append(os.Environ(), pipeServerEnv+"="+pipe, pipeReportEnv+"="+report)
	var said bytes.Buffer
	server.Stdout, server.Stderr = &said, &said
	require.NoError(t, server.Start(), "the serving half of this test")
	// Waited for here as well as below, so what it said is readable — and so
	// this is not left running — however the assertions below turn out.
	t.Cleanup(func() {
		_ = server.Wait()
		t.Logf("the serving half said:\n%s", said.String())
	})
	require.Eventually(t, func() bool { _, err := os.Stat(readyBeside(report)); return err == nil },
		10*time.Second, 10*time.Millisecond, "the serving half never got its pipe up")

	reachable := PipeProber{Timeout: 5 * time.Second}.Reachable(t.Context(), pipe)

	require.NoError(t, server.Wait(), "the serving half must have finished saying what it was offered")
	require.True(t, reachable, "an agent answered on that pipe, so the probe must say so")
	raw, err := os.ReadFile(report)
	require.NoError(t, err, "what the serving half was offered")
	var offer impersonationOffer
	require.NoError(t, json.Unmarshal(raw, &offer), "what the serving half was offered")
	require.Empty(t, offer.Refused, "a server may ask which account is asking")
	assert.Equal(t, uint32(windows.SecurityIdentification), offer.Level,
		"a server may find out which account is asking and go no further; %d lets it act as that account",
		offer.Level)
	assert.False(t, offer.ActedAsUs,
		"and it must not be able to open anything as that account, which is what the level decides")
}

// sidOf is a real account on every Windows machine, named the way this system
// names accounts.
func sidOf(t *testing.T, known windows.WELL_KNOWN_SID_TYPE) string {
	t.Helper()

	sid, err := windows.CreateWellKnownSid(known)
	require.NoError(t, err, "a well-known account this system must know")
	return sid.String()
}

// F58: the pipe namespace is the machine's, and the name an agent is expected
// on is one any account can hold while no agent has it. Answering the agent's
// own handshake is something a stranger can do perfectly; being the account
// whose agent it would be is not.
func TestAnEndpointHeldBySomebodyElseIsNotYourAgent(t *testing.T) {
	// A table naming one real account, and not the one this test is running as.
	held := PipeProber{Timeout: 2 * time.Second, trustedOwners: []string{sidOf(t, windows.WinLocalServiceSid)}}

	assert.False(t, held.Reachable(t.Context(), fakeAgentPipe(t, pipeReplyIdentities(2))),
		"it answered the agent's own handshake, and it is still not an agent of yours")
}

// F58: and the refusal must not fall on the machine it is there to protect —
// the accounts an endpoint may belong to include the one asking, whose own
// agent it would be.
func TestAnEndpointThisAccountIsServingIsYourAgent(t *testing.T) {
	assert.True(t, PipeProber{Timeout: 2 * time.Second}.Reachable(t.Context(), fakeAgentPipe(t, pipeReplyIdentities(0))),
		"this test made that pipe, so the accounts an endpoint may belong to must include the one that did")
}

// F58: the endpoint this platform's own agent is served on belongs to the
// system, not to anybody logged in. A table that failed to name the system
// would refuse every real agent on this platform while every other test in
// this file went on passing, since they all serve their own pipes.
func TestTheEndpointThisSystemsOwnAgentIsServedOnIsOneToSpeakOn(t *testing.T) {
	served, err := openPipe(agent.SystemEndpoint().Native())
	if err != nil {
		t.Skip("nothing is serving this system's own agent endpoint here")
	}
	t.Cleanup(func() { _ = windows.CloseHandle(served) })

	owner, err := ownerOf(served)

	require.NoError(t, err, "who owns the endpoint this system's own agent is served on")
	assert.Contains(t, ownersWhoseAgentThisCouldBe(), owner,
		"the agent this system serves itself must be one SSHakku will speak to")
}

// serveAndWriteDownWhatWasOffered is the serving half of the test above,
// running as a child of it: one connection on the pipe it was named, answered
// as an agent answers, and on the way an attempt to become whoever connected.
func serveAndWriteDownWhatWasOffered(t *testing.T, pipe, report string) {
	t.Helper()

	offers := make(chan impersonationOffer, 1)
	fakeAgentPipeNamed(t, pipe, pipeReportImpersonationLevel(offers))
	require.NoError(t, os.WriteFile(readyBeside(report), nil, 0o600), "saying the pipe is up")

	select {
	case offer := <-offers:
		raw, err := json.Marshal(offer)
		require.NoError(t, err, "what this half was offered")
		require.NoError(t, os.WriteFile(report, raw, 0o600), "what this half was offered")
	case <-t.Context().Done():
		require.Fail(t, "nobody connected to the pipe this half was asked to serve")
	}
}
