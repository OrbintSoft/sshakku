//go:build windows

package reach

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"golang.org/x/sys/windows"
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
// Like the socket fake above it asserts nothing: it runs on a goroutine of its
// own, where an assertion would report from outside the test's goroutine. What
// it serves is the subject's input, not its verdict.
func fakeAgentPipe(t *testing.T, reply func(io.ReadWriter)) string {
	t.Helper()
	name := pipeName(t)
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
				_ = knock.Close()
			}
			<-served
		}
		_ = f.Close()
	})
	return name
}

// pipeReplyIdentities answers a request with an identities-answer listing nkeys
// keys; the keys themselves are omitted, since the prober reads only the type.
func pipeReplyIdentities(nkeys uint32) func(io.ReadWriter) {
	return func(rw io.ReadWriter) {
		drainPipeRequest(rw)
		payload := []byte{msgIdentitiesAnswer, 0, 0, 0, 0}
		binary.BigEndian.PutUint32(payload[1:], nkeys)
		frame := make([]byte, 4+len(payload))
		binary.BigEndian.PutUint32(frame, uint32(len(payload)))
		copy(frame[4:], payload)
		_, _ = rw.Write(frame)
	}
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
		wrongType := func(rw io.ReadWriter) {
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
	silent := func(rw io.ReadWriter) {
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
	silent := func(rw io.ReadWriter) {
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
