//go:build unix

package prompt

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// saveTTYSeams snapshots the terminal seams and restores them when the
// (sub)test ends.
func saveTTYSeams(t *testing.T) {
	t.Helper()
	oOpen, oGet, oSet := openTTY, getTermios, setTermios
	t.Cleanup(func() { openTTY, getTermios, setTermios = oOpen, oGet, oSet })
}

// echoState records what the terminal's echo flag was set to, in order. A
// terminal starts out echoing, so the recorder starts from a Termios with ECHO
// on — the state a real one is in when the prompt is about to be written.
type echoState struct{ set []bool }

// on reports whether echo was on for each call, oldest first.
func (e *echoState) on() []bool { return e.set }

// fakeTTYPair points openTTY at one end of a socketpair — a bidirectional,
// blocking channel that stands in for /dev/tty — and stubs the echo ioctls. It
// returns the other end for the test to feed input to and read the written
// prompt from.
func fakeTTYPair(t *testing.T) *os.File {
	t.Helper()
	peer, _ := fakeTTYPairRecordingEcho(t)
	return peer
}

// fakeTTYPairRecordingEcho is fakeTTYPair for a test whose subject is the echo
// itself: it also hands back what the terminal's echo flag was set to. The
// ioctls have to be replaced — there is no terminal here to run them against —
// but which way they were driven is exactly what must not be assumed.
func fakeTTYPairRecordingEcho(t *testing.T) (*os.File, *echoState) {
	t.Helper()
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	require.NoError(t, err, "a bidirectional channel to stand in for /dev/tty")
	ttyEnd := os.NewFile(uintptr(fds[0]), "tty")
	peer := os.NewFile(uintptr(fds[1]), "peer")
	t.Cleanup(func() { _ = ttyEnd.Close(); _ = peer.Close() })

	echo := &echoState{}
	saveTTYSeams(t)
	openTTY = func() (*os.File, error) { return ttyEnd, nil }
	getTermios = func(int, uint) (*unix.Termios, error) { return &unix.Termios{Lflag: unix.ECHO}, nil }
	setTermios = func(_ int, _ uint, t *unix.Termios) error {
		echo.set = append(echo.set, t.Lflag&unix.ECHO != 0)
		return nil
	}
	return peer, echo
}

// TestReadTTYLineTurnsTheEchoOffAndBackOn is what asking for a passphrase on a
// terminal amounts to: the characters must not appear as they are typed. The
// terminal echoes each one as it arrives, so the flag has to be off before the
// prompt is even written — anything typed, pasted or piped in between the two
// would otherwise be printed in the clear — and back on afterwards, or the shell
// the user returns to is one they cannot see themselves typing in.
func TestReadTTYLineTurnsTheEchoOffAndBackOn(t *testing.T) {
	peer, echo := fakeTTYPairRecordingEcho(t)
	_, err := peer.WriteString("s3cr3t\n")
	require.NoError(t, err, "the user types a passphrase")

	_, err = ReadTTYLine("Enter passphrase: ", true)
	require.NoError(t, err, "reading it must succeed")
	assert.Equal(t, []bool{false, true}, echo.on(),
		"the echo goes off before anything is asked, and back on before the terminal is handed back")
}

// TestReadTTYLineLeavesAnEchoedPromptAlone is the other half: a host-key
// confirmation is answered by typing "yes", and a user who cannot see what they
// are typing cannot answer it.
func TestReadTTYLineLeavesAnEchoedPromptAlone(t *testing.T) {
	peer, echo := fakeTTYPairRecordingEcho(t)
	_, err := peer.WriteString("yes\n")
	require.NoError(t, err, "the user types an answer")

	_, err = ReadTTYLine("Are you sure? ", false)
	require.NoError(t, err, "reading it must succeed")
	assert.Empty(t, echo.on(), "a question that is not a secret must leave the terminal exactly as it was found")
}

func TestReadTTYLineReadsSecretLine(t *testing.T) {
	peer := fakeTTYPair(t)
	_, err := peer.WriteString("s3cr3t\n")
	require.NoError(t, err, "the user types a passphrase")
	got, err := ReadTTYLine("Enter passphrase: ", true)
	require.NoError(t, err, "reading it must succeed")
	assert.Equal(t, "s3cr3t", got, "and it must be what they typed, without the newline that ended the line")
}

func TestReadTTYLineReadsEchoedLine(t *testing.T) {
	peer := fakeTTYPair(t)
	_, err := peer.WriteString("plain\n")
	require.NoError(t, err, "the user types an answer")
	got, err := ReadTTYLine("Answer: ", false)
	require.NoError(t, err, "reading it must succeed")
	assert.Equal(t, "plain", got, "and it must be what they typed")
}

// TestReadTTYLineEndOfInputIsARefusal covers the terminal half of F38 where a
// pty is not available: input that ends without an answer is the user turning
// the question down, which the caller has to be able to tell apart from the
// terminal failing them.
func TestReadTTYLineEndOfInputIsARefusal(t *testing.T) {
	peer := fakeTTYPair(t)
	// Shut only the far end's write side (not the whole socket): the prompt
	// write still succeeds, but the read hits end-of-input with an empty line,
	// which is what Ctrl-D produces on a real terminal. Closing the peer
	// outright would instead break the prompt write and exercise a different
	// branch.
	require.NoError(t, unix.Shutdown(int(peer.Fd()), unix.SHUT_WR), "the user presses Ctrl-D")
	_, err := ReadTTYLine("Answer: ", false)
	assert.ErrorIs(t, err, ErrCanceled,
		"input that ended without an answer is the user turning the question down, not the terminal failing them")
}

// TestReadTTYLineReadErrorOnEmpty covers the other empty-line ending: the read
// itself failing, which is the terminal letting the user down rather than the
// user declining, and must not be reported as a refusal.
func TestReadTTYLineReadErrorOnEmpty(t *testing.T) {
	saveTTYSeams(t)
	// A write-only handle takes the prompt and then fails the read, which no
	// closed input can produce.
	path := filepath.Join(t.TempDir(), "wo")
	require.NoError(t, os.WriteFile(path, nil, 0o600), "a handle that takes the prompt and then fails the read")
	openTTY = func() (*os.File, error) { return os.OpenFile(path, os.O_WRONLY, 0) }
	_, err := ReadTTYLine("Answer: ", false)
	require.Error(t, err, "a terminal that could not be read must be reported")
	assert.NotErrorIs(t, err, ErrCanceled,
		"but not as a refusal: the terminal let the user down, and they never got to decline anything")
}

func TestReadTTYLineOpenError(t *testing.T) {
	saveTTYSeams(t)
	cause := errors.New("open /dev/tty: no such device")
	openTTY = func() (*os.File, error) { return nil, cause }
	_, err := ReadTTYLine("Answer: ", false)
	assert.ErrorIs(t, err, ErrNoTerminal,
		"having no terminal is an ordinary state the callers fall back from, told apart by this very error")
	assert.ErrorIs(t, err, cause,
		"and why the open failed stays reachable: a session that has no terminal at all and one whose "+
			"terminal could not be opened are different situations, and the sentinel alone cannot separate them")
}

func TestReadTTYLinePromptWriteError(t *testing.T) {
	saveTTYSeams(t)
	// A read-only handle makes the prompt write fail, which is neither an open
	// failure nor a read failure.
	path := filepath.Join(t.TempDir(), "ro")
	require.NoError(t, os.WriteFile(path, nil, 0o600), "a handle the prompt cannot be written to")
	openTTY = func() (*os.File, error) { return os.OpenFile(path, os.O_RDONLY, 0) }
	_, err := ReadTTYLine("Answer: ", false)
	require.Error(t, err, "a question that never appeared cannot be waited on for an answer")
	assert.NotErrorIs(t, err, ErrCanceled,
		"and not as a refusal: the user was never asked, so they declined nothing, and the caller would stop "+
			"asking about a key they still want")
}

func TestReadTTYLineDisableEchoGetError(t *testing.T) {
	_ = fakeTTYPair(t)
	getTermios = func(int, uint) (*unix.Termios, error) { return nil, errors.New("ENOTTY") }
	_, err := ReadTTYLine("Passphrase: ", true)
	assert.Error(t, err,
		"a terminal whose echo could not be turned off must not be asked for a passphrase in the clear")
}

func TestReadTTYLineDisableEchoSetError(t *testing.T) {
	_ = fakeTTYPair(t)
	setTermios = func(int, uint, *unix.Termios) error { return errors.New("EINVAL") }
	_, err := ReadTTYLine("Passphrase: ", true)
	assert.Error(t, err,
		"a terminal whose echo could not be turned off must not be asked for a passphrase in the clear")
}

func TestTTYPrompterPromptAndAvailable(t *testing.T) {
	assert.True(t, TTYPrompter{}.Available(t.Context()),
		"the terminal is where the question goes when nothing else can take it, so it never declines the job")

	peer := fakeTTYPair(t)
	_, err := peer.WriteString("keypass\n")
	require.NoError(t, err, "the user types a passphrase")
	got, err := TTYPrompter{}.Prompt(t.Context(), "id_rsa")
	require.NoError(t, err, "reading it must succeed")
	assert.Equal(t, "keypass", got, "and it must be what they typed")
}
