//go:build unix

package keys

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

// saveTTYSeams snapshots the terminal seams and restores them when the
// (sub)test ends.
func saveTTYSeams(t *testing.T) {
	t.Helper()
	oOpen, oGet, oSet := openTTY, getTermios, setTermios
	t.Cleanup(func() { openTTY, getTermios, setTermios = oOpen, oGet, oSet })
}

// fakeTTYPair points openTTY at one end of a socketpair — a bidirectional,
// blocking channel that stands in for /dev/tty — and stubs the echo ioctls to
// succeed. It returns the other end for the test to feed input to and read the
// written prompt from.
func fakeTTYPair(t *testing.T) *os.File {
	t.Helper()
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatalf("socketpair: %v", err)
	}
	ttyEnd := os.NewFile(uintptr(fds[0]), "tty")
	peer := os.NewFile(uintptr(fds[1]), "peer")
	t.Cleanup(func() { _ = ttyEnd.Close(); _ = peer.Close() })

	saveTTYSeams(t)
	openTTY = func() (*os.File, error) { return ttyEnd, nil }
	getTermios = func(int, uint) (*unix.Termios, error) { return &unix.Termios{}, nil }
	setTermios = func(int, uint, *unix.Termios) error { return nil }
	return peer
}

func TestReadTTYLineReadsSecretLine(t *testing.T) {
	peer := fakeTTYPair(t)
	if _, err := peer.WriteString("s3cr3t\n"); err != nil {
		t.Fatalf("feed input: %v", err)
	}
	got, err := ReadTTYLine("Enter passphrase: ", true)
	if err != nil {
		t.Fatalf("ReadTTYLine: %v", err)
	}
	if got != "s3cr3t" {
		t.Fatalf("ReadTTYLine = %q, want %q", got, "s3cr3t")
	}
}

func TestReadTTYLineReadsEchoedLine(t *testing.T) {
	peer := fakeTTYPair(t)
	if _, err := peer.WriteString("plain\n"); err != nil {
		t.Fatalf("feed input: %v", err)
	}
	got, err := ReadTTYLine("Answer: ", false)
	if err != nil {
		t.Fatalf("ReadTTYLine: %v", err)
	}
	if got != "plain" {
		t.Fatalf("ReadTTYLine = %q, want %q", got, "plain")
	}
}

func TestReadTTYLineReadErrorOnEmpty(t *testing.T) {
	peer := fakeTTYPair(t)
	// Shut only the far end's write side (not the whole socket): the prompt
	// write still succeeds, but the read hits EOF with an empty line, which
	// surfaces as the read error. Closing the peer outright would instead break
	// the prompt write and exercise a different branch.
	if err := unix.Shutdown(int(peer.Fd()), unix.SHUT_WR); err != nil {
		t.Fatalf("shutdown peer write side: %v", err)
	}
	if _, err := ReadTTYLine("Answer: ", false); err == nil {
		t.Fatal("ReadTTYLine returned nil error, want the read EOF error on an empty line")
	}
}

func TestReadTTYLineOpenError(t *testing.T) {
	saveTTYSeams(t)
	openTTY = func() (*os.File, error) { return nil, errors.New("open /dev/tty: no such device") }
	_, err := ReadTTYLine("Answer: ", false)
	if !errors.Is(err, ErrNoTerminal) {
		t.Fatalf("ReadTTYLine error = %v, want ErrNoTerminal", err)
	}
}

func TestReadTTYLinePromptWriteError(t *testing.T) {
	saveTTYSeams(t)
	// A read-only handle makes the prompt write fail, which is neither an open
	// failure nor a read failure.
	path := filepath.Join(t.TempDir(), "ro")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	openTTY = func() (*os.File, error) { return os.OpenFile(path, os.O_RDONLY, 0) }
	if _, err := ReadTTYLine("Answer: ", false); err == nil {
		t.Fatal("ReadTTYLine returned nil error, want the prompt-write failure")
	}
}

func TestReadTTYLineDisableEchoGetError(t *testing.T) {
	_ = fakeTTYPair(t)
	getTermios = func(int, uint) (*unix.Termios, error) { return nil, errors.New("ENOTTY") }
	if _, err := ReadTTYLine("Passphrase: ", true); err == nil {
		t.Fatal("ReadTTYLine returned nil error, want the termios-get failure")
	}
}

func TestReadTTYLineDisableEchoSetError(t *testing.T) {
	_ = fakeTTYPair(t)
	setTermios = func(int, uint, *unix.Termios) error { return errors.New("EINVAL") }
	if _, err := ReadTTYLine("Passphrase: ", true); err == nil {
		t.Fatal("ReadTTYLine returned nil error, want the termios-set failure")
	}
}

func TestTTYPrompterPromptAndAvailable(t *testing.T) {
	if !(TTYPrompter{}).Available() {
		t.Error("TTYPrompter.Available() = false, want true")
	}

	peer := fakeTTYPair(t)
	if _, err := peer.WriteString("keypass\n"); err != nil {
		t.Fatalf("feed input: %v", err)
	}
	got, err := TTYPrompter{}.Prompt("id_rsa")
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if got != "keypass" {
		t.Fatalf("Prompt = %q, want %q", got, "keypass")
	}
}
